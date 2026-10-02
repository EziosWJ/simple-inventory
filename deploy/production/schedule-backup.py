#!/usr/bin/env python3
"""Install/remove one project-owned daily cron entry without changing other jobs."""
import argparse
import os
from pathlib import Path
import shlex
import subprocess
import sys
import time
from manage import Deployment

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--env', required=True)
parser.add_argument('--directory', required=True)
parser.add_argument('--remove', action='store_true')
args = parser.parse_args()
os.umask(0o077)
deployment = Deployment(args.env)
root = Path(args.directory).resolve()
root.mkdir(mode=0o700, parents=True, exist_ok=True)
if root.stat().st_mode & 0o077:
    raise SystemExit('Backup directory must have mode 700')
marker = '# simple-inventory-backup:' + deployment.project
current = subprocess.run(['crontab', '-l'], capture_output=True, text=True)
if current.returncode not in (0, 1) or (current.returncode == 1 and 'no crontab' not in current.stderr.lower()):
    raise SystemExit('Unable to read current crontab; existing jobs are preserved')
lines = [line for line in current.stdout.splitlines() if not line.endswith(marker)]
if not args.remove:
    command = [sys.executable, str(Path(__file__).with_name('manage.py')), '--env', str(deployment.env), 'backup', '--directory', str(root)]
    job = shlex.join(command) + ' >> ' + shlex.quote(str(root / 'backup.log')) + ' 2>&1'
    # Cron interprets unescaped percent even inside shell quotes.
    lines.append('15 2 * * * ' + job.replace('%', r'\%') + ' ' + marker)
subprocess.run(['crontab', '-'], input='\n'.join(lines) + '\n', text=True, check=True)
print(('REMOVED' if args.remove else 'SCHEDULED daily 02:15 in server local timezone ' + '/'.join(time.tzname)) + ': ' + deployment.project)
