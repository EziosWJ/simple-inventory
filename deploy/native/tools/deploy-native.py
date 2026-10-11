#!/usr/bin/env python3
import grp
import os
from pathlib import Path
import pwd
import shutil
import subprocess as sp
import sys
import time
import urllib.parse
import urllib.request
import yaml
from native_common import BASE, lock, restore_owners, run, snapshot, versions

release = BASE / 'releases' / sys.argv[2]
unit = Path('/etc/systemd/system/simple-inventory.service')
env = {**os.environ, 'APP_ENV': 'prod'}

def migrate(*args):
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
if not release.is_dir() or release.parent != BASE / 'releases':
    raise SystemExit('Invalid release')
run(['sha256sum', '--quiet', '-c', 'SHA256SUMS'], cwd=release)

if sys.argv[1] == 'prepare':
    try:
        account = pwd.getpwnam('simple-inventory')
    except KeyError:
        run(['useradd', '--system', '--user-group', '--home-dir', '/nonexistent', '--no-create-home', '--shell', '/usr/sbin/nologin', 'simple-inventory'])
        account = pwd.getpwnam('simple-inventory')
    os.chown(BASE / 'releases', 0, account.pw_gid)
    (BASE / 'releases').chmod(0o750)
    for p in [release, *release.rglob('*')]:
        if not p.is_symlink():
            os.chown(p, 0, 0)
    cfgdir = BASE / 'native-config'
    cfgdir.mkdir(mode=0o750, exist_ok=True)
    os.chown(cfgdir, 0, account.pw_gid)
    cfgdir.chmod(0o750)
    cfg = yaml.safe_load((BASE / 'config/config.prod.yaml').read_text())
    oldurl = cfg['database']['url']
    u = urllib.parse.urlsplit(oldurl)
    if u.hostname != 'postgresql' or u.port != 5432 or u.path != '/jxc':
        raise ValueError('Unexpected database URL; requires review')
    cfg['database']['url'] = urllib.parse.urlunsplit((u.scheme, '127.0.0.1:5432', u.path, u.query, u.fragment))
    cfg.setdefault('http', {}).update(address='127.0.0.1:18080', trusted_proxies=['127.0.0.1'])
    cfg.setdefault('file', {})['storage_root'] = str(BASE / 'data/uploads')
    cfg.setdefault('swagger', {})['enabled'] = False
    out = cfgdir / 'config.prod.yaml'
    with open(out, 'w', opener=lambda path, flags: os.open(path, flags, 0o640)) as f:
        yaml.safe_dump(cfg, f, allow_unicode=True, sort_keys=False)
    os.chown(out, 0, account.pw_gid)
    out.chmod(0o640)
    link = release / 'configs/config.prod.yaml'
    if not link.is_symlink():
        link.symlink_to(out)
    elif link.resolve() != out:
        raise ValueError('Unexpected configuration link')
    migrate('check', '--kind', 'all')
    stageuploads = release / 'preflight-uploads'
    stageuploads.mkdir(mode=0o750, exist_ok=True)
    os.chown(stageuploads, account.pw_uid, account.pw_gid)
    cmd = ['systemd-run', '--unit=simple-inventory-preflight', '--property=User=simple-inventory', '--property=Group=simple-inventory', '--property=WorkingDirectory=' + str(release), '--setenv=APP_ENV=prod', '--setenv=APP_HTTP__ADDRESS=127.0.0.1:18081', '--setenv=APP_FILE__STORAGE_ROOT=' + str(stageuploads), str(release / 'simple-inventory-api')]
    try:
        sp.run(['systemctl', 'reset-failed', 'simple-inventory-preflight'], stdout=sp.DEVNULL, stderr=sp.DEVNULL)
        run(cmd)
        ready(18081)
        page = urllib.request.urlopen('http://127.0.0.1:18081/').read()
        if b'assets/index-' not in page:
            raise RuntimeError('Frontend asset reference absent')
        print('PREFLIGHT_OK', release.name, flush=True)
    finally:
        run(['systemctl', 'stop', 'simple-inventory-preflight'])

elif sys.argv[1] == 'switch':
    guard = lock()
    migrate('check', '--kind', 'all')
    before = versions()
    backup = None
    changed_schema = False
    try:
        run(['systemctl', 'stop', 'simple-inventory.service'])
        backup = snapshot('before-' + release.name)
        print('BACKUP_COMPLETE', backup, flush=True)
        migrate('up', '--kind', 'all')
        changed_schema = versions() != before
        if changed_schema:
            raise RuntimeError('Unexpected schema change; keep application stopped')
        migrate('check', '--kind', 'all')
        account = pwd.getpwnam('simple-inventory')
        root = BASE / 'data/uploads'
        for p in [root, *root.rglob('*')]:
            os.chown(p, account.pw_uid, account.pw_gid, follow_symlinks=False)
        run(['runuser', '-u', 'simple-inventory', '--', 'python3', '-c', 'from pathlib import Path; import tempfile; p=Path("/opt/simple-inventory/data/uploads"); [f.open("rb").close() for f in p.rglob("*") if f.is_file()]; f=tempfile.TemporaryFile(dir=p); f.write(b"permission-check"); f.close()'])
        if (BASE / 'current').exists() or (BASE / 'current').is_symlink():
            raise ValueError('Existing current link requires upgrade-specific handling')
        (BASE / 'current').symlink_to(release)
        unit.write_text('''[Unit]
Description=Simple Inventory native application
After=network-online.target docker.service
Wants=network-online.target docker.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=simple-inventory
Group=simple-inventory
WorkingDirectory=/opt/simple-inventory/current
Environment=APP_ENV=prod
Environment=GIN_MODE=release
ExecStartPre=/opt/simple-inventory/current/simple-inventory-migrate check --kind all
ExecStart=/opt/simple-inventory/current/simple-inventory-api
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/simple-inventory/data/uploads
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
''')
        unit.chmod(0o644)
        run(['systemd-analyze', 'verify', unit])
        run(['systemctl', 'daemon-reload'])
        run(['systemctl', 'enable', '--now', 'simple-inventory.service'])
        ready(18080)
        run(['curl', '--fail', '--silent', '--show-error', '--max-time', '15', 'https://jxc.ezios.cn/ready'])
        if versions() != before:
            raise RuntimeError('Migration state changed during switch')
        (BASE / 'native-tools/initial-backup.txt').write_text(str(backup) + '\n')
        print('\nDEPLOYMENT_OK', release.name, flush=True)
    except BaseException:
        # Never restart old code against an upgraded schema.
        if not changed_schema and versions() == before:
            sp.run(['systemctl', 'stop', 'simple-inventory.service'], check=False)
            if backup:
                shutil.copy2(backup / 'simple-inventory.service', unit)
                restore_owners(backup)
            run(['systemctl', 'daemon-reload'])
            run(['systemctl', 'start', 'simple-inventory.service'])
            print('ROLLED_BACK_TO_DOCKER', flush=True)
        else:
            print('SCHEMA_CHANGED_KEEP_STOPPED', flush=True)
        raise
else:
    raise SystemExit('Use prepare or switch')

