#!/usr/bin/env python3
"""Upgrade the running native deployment to another prepared release.

Sequence: take the shared lock, verify the release checksums, run a read-only
migration check, stop the service, write a coherent snapshot (database,
uploads, private config, unit file and the current release), run the migration
explicitly, refuse to start when the migration set changed, repoint `current`,
start the service and wait for /ready both on the loopback port and through
https://jxc.ezios.cn.

A failure before the migration set changes restores the previous `current`
link and starts the old release again; the old releases are never deleted.
Use deploy-native.py prepare <release> first, then this tool.
"""
import os
from pathlib import Path
import shutil
import subprocess as sp
import sys
import time
import urllib.request

from native_common import BASE, lock, run, snapshot, versions

unit = Path('/etc/systemd/system/simple-inventory.service')
env = {**os.environ, 'APP_ENV': 'prod'}


def migrate(release, *args):
    run([release / 'simple-inventory-migrate', *args], cwd=release, env=env)


def ready(port):
    for _ in range(40):
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{port}/ready', timeout=2) as r:
                if r.status == 200:
                    return
        except Exception:
            time.sleep(1)
    raise RuntimeError('Readiness timeout')


if os.geteuid() != 0:
    raise SystemExit('Run with sudo')
if len(sys.argv) != 2:
    raise SystemExit('Use: upgrade-native.py <release directory name>')
release = BASE / 'releases' / sys.argv[1]
if not release.is_dir() or release.parent != BASE / 'releases':
    raise SystemExit('Invalid release')
link = BASE / 'current'
if not link.is_symlink():
    raise SystemExit('No current release; use deploy-native.py switch for the first deployment')
previous = link.resolve()
if previous == release:
    raise SystemExit('Release is already current')
if not (release / 'configs/config.prod.yaml').is_symlink():
    raise SystemExit('Release was not prepared; run deploy-native.py prepare first')
run(['sha256sum', '--quiet', '-c', 'SHA256SUMS'], cwd=release)

guard = lock()
migrate(release, 'check', '--kind', 'all')
before = versions()
print('VERSIONS_BEFORE\n' + before, flush=True)
backup = None
changed_schema = False
try:
    run(['systemctl', 'stop', 'simple-inventory.service'])
    backup = snapshot('before-' + release.name)
    print('BACKUP_COMPLETE', backup, flush=True)
    migrate(release, 'up', '--kind', 'all')
    changed_schema = versions() != before
    if changed_schema:
        raise RuntimeError('Migration set changed; review the upgrade before starting the new release')
    migrate(release, 'check', '--kind', 'all')
    unit_before = unit.read_bytes()
    link.unlink()
    link.symlink_to(release)
    run(['systemd-analyze', 'verify', unit])
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'start', 'simple-inventory.service'])
    ready(18080)
    run(['curl', '--fail', '--silent', '--show-error', '--max-time', '15', 'https://jxc.ezios.cn/ready'])
    if versions() != before:
        raise RuntimeError('Migration state changed during the upgrade')
    (BASE / 'native-tools/last-upgrade.txt').write_text(
        f'current={release.name}\nprevious={previous.name}\nbackup={backup}\nunit_sha256_unchanged={unit.read_bytes() == unit_before}\n')
    print('\nUPGRADE_OK', release.name, 'from', previous.name, flush=True)
except BaseException:
    if not changed_schema:
        sp.run(['systemctl', 'stop', 'simple-inventory.service'], check=False)
        if link.is_symlink():
            link.unlink()
        link.symlink_to(previous)
        if backup:
            shutil.copy2(backup / 'simple-inventory.service', unit)
        run(['systemctl', 'daemon-reload'])
        run(['systemctl', 'start', 'simple-inventory.service'])
        print('ROLLED_BACK_TO', previous.name, flush=True)
    else:
        print('SCHEMA_CHANGED_KEEP_STOPPED', flush=True)
    raise
