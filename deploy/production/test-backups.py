#!/usr/bin/env python3
"""Destructive fixtures only inside an explicitly named phase5-* isolated backup directory."""
import argparse
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import shutil
import subprocess as sp
import sys
from manage import Deployment, backup, digest, verify

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--env', required=True)
parser.add_argument('--directory', required=True)
args = parser.parse_args()
os.umask(0o077)
d = Deployment(args.env)
assert d.project.startswith('phase5-'), 'Only isolated phase5-* projects may run this test'
root = Path(args.directory).resolve()
backup(d, root)
complete = lambda: [p for p in root.iterdir() if p.name.startswith('inventory-') and (p / 'COMPLETE').is_file()]
first = max(complete(), key=lambda p: verify(p)['created_at'])
assert first.stat().st_mode & 0o077 == 0
assert all(p.stat().st_mode & 0o077 == 0 for p in first.iterdir())
command = [sys.executable, str(Path(__file__).with_name('manage.py')), '--env', str(d.env), 'backup', '--directory', str(root)]
with (root / '.backup.lock').open('a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    result = sp.run(command, capture_output=True)
    assert result.returncode != 0 and b'Another backup is running' in result.stderr
assert len(complete()) == 1
print('PASS overlapping invocation rejected without losing a backup')

# Fail the real upload archive helper after pg_dump succeeds, then verify real service resumption.
fault = root / '.fault-injection'; fault.mkdir(mode=0o700)
docker = shutil.which('docker')
wrapper = fault / 'docker'
wrapper.write_text('#!/bin/sh\ncase "$*" in *"--entrypoint /bin/tar"*) exit 42;; esac\nexec "' + docker + '" "$@"\n')
wrapper.chmod(0o700)
try:
    result = sp.run(command, env={**os.environ, 'PATH': str(fault) + os.pathsep + os.environ['PATH']}, capture_output=True)
    assert result.returncode != 0
    assert len(complete()) == 1
    verify(first)
    running = d.call('ps', '--services', '--status', 'running', capture_output=True).stdout.decode().split()
    assert {'api', 'proxy'} <= set(running)
finally:
    shutil.rmtree(fault)
print('PASS partial archive failure returns nonzero, keeps latest complete backup and resumes real API/proxy')

def fixture(name, project=None, corrupt=False):
    target = root / name
    shutil.copytree(first, target)
    manifest = json.loads((target / 'manifest.json').read_text())
    manifest['created_at'] = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=40)).isoformat()
    if project: manifest['project'] = project
    (target / 'manifest.json').write_text(json.dumps(manifest, indent=2))
    (target / 'COMPLETE').write_text(digest(target / 'manifest.json') + '\n')
    if corrupt:
        with (target / 'uploads.tar').open('ab') as stream: stream.write(b'corrupted')
    return target

expired = fixture('inventory-' + d.project + '-expired-fixture')
other = fixture('inventory-other-project-fixture', 'other-project')
corrupt = fixture('inventory-' + d.project + '-corrupt-fixture', corrupt=True)
unknown = root / ('inventory-' + d.project + '-unknown-fixture'); unknown.mkdir()
note = root / 'unrelated.txt'; note.write_text('Keep this unknown file')
backup(d, root)
assert not expired.exists()
assert other.exists() and corrupt.exists() and unknown.exists() and note.exists() and first.exists()
assert len([p for p in complete() if p.name.startswith('inventory-' + d.project + '-') and p != corrupt]) == 2
print('PASS 30-day retention removes only verified expired own backups; latest/prior/other/corrupt/unknown remain')

# Restore validates before touching the target project or creating a database.
target = root / 'must-not-be-created'
result = sp.run([sys.executable, str(Path(__file__).with_name('manage.py')), 'restore', str(corrupt),
                 '--target', str(target), '--project', d.project + '-bad-restore', '--http-port', '18090',
                 '--https-port', '18490', '--subnet', '172.30.25.0/24', '--proxy-ip', '172.30.25.3'], capture_output=True)
assert result.returncode != 0 and not target.exists()
print('PASS corrupted restore rejected before creating destination or changing original deployment')
