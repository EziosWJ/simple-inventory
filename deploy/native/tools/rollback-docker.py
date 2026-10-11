#!/usr/bin/env python3
import os
import shutil
from native_common import BASE, lock, restore_owners, run, verify

if os.geteuid() != 0:
    raise SystemExit('Run with sudo')
guard = lock()
from pathlib import Path
p = Path((BASE / 'native-tools/initial-backup.txt').read_text().strip())
verify(p)
# Check retained old image against current database before stopping native API.
run(['docker', 'compose', '--env-file', BASE / '.env', '-f', BASE / 'compose.yaml', 'run', '--rm', '--no-deps', 'migrate', 'check', '--kind', 'all'])
run(['systemctl', 'disable', '--now', 'simple-inventory-backup.timer'])
run(['systemctl', 'stop', 'simple-inventory.service'])
try:
    restore_owners(p)
    shutil.copy2(p / 'simple-inventory.service', '/etc/systemd/system/simple-inventory.service')
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'start', 'simple-inventory.service'])
except BaseException:
    raise SystemExit('Rollback incomplete; keep maintenance and inspect service')
print('DOCKER_ROLLBACK_STARTED; verify /ready and HTTPS')
