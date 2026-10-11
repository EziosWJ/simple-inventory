#!/usr/bin/env python3
import os
from native_common import BASE, lock, run, snapshot

if os.geteuid() != 0:
    raise SystemExit('Run with sudo')
guard = lock()
run(['systemctl', 'is-active', '--quiet', 'simple-inventory.service'])
if 'Docker Compose' in run(['systemctl', 'show', '-p', 'Description', '--value', 'simple-inventory.service'], capture_output=True, text=True).stdout:
    raise SystemExit('Daily backup requires native service')
try:
    run(['systemctl', 'stop', 'simple-inventory.service'])
    result = snapshot('daily')
finally:
    run(['systemctl', 'start', 'simple-inventory.service'])
print('BACKUP_COMPLETE', result)

