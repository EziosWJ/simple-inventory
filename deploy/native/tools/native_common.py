import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess as sp
import tarfile

BASE = Path('/opt/simple-inventory')
DB = '1Panel-postgresql-NPcN'

def run(args, **kw):
    kw.setdefault('timeout', 300)
    return sp.run([str(a) for a in args], check=True, **kw)

def dbcmd(command, *args):
    return ['docker', 'exec', '-i', DB, 'sh', '-ec', command, 'sh', *args]

def versions():
    q = 'SELECT version_id,is_applied FROM goose_schema_db_version ORDER BY id; SELECT version_id,is_applied FROM goose_seed_db_version ORDER BY id;'
    return run(dbcmd('psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -At', 'jxc'), input=q.encode(), capture_output=True).stdout.decode()

def lock():
    f = (BASE / 'backups/.native.lock').open('a')
    fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
    return f

def digest(p):
    h = hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda: f.read(1024*1024), b''):
            h.update(b)
    return h.hexdigest()

def snapshot(label):
    os.umask(0o077)
    p = BASE / 'backups' / ('native-' + label + '-' + dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ'))
    p.mkdir(mode=0o700)
    with (p / 'database.dump').open('wb') as f:
        run(dbcmd('pg_dump -U "$POSTGRES_USER" -d "$1" --format=custom --no-owner --no-acl', 'jxc'), stdout=f)
    with (p / 'database.dump').open('rb') as f:
        run(['docker', 'exec', '-i', DB, 'pg_restore', '--list'], stdin=f, stdout=sp.DEVNULL)
    root = BASE / 'data/uploads'
    owners = {}
    for item in [root, *root.rglob('*')]:
        if item.is_symlink():
            raise ValueError('Upload symlink requires manual review')
        st = item.stat()
        owners[str(item.relative_to(root))] = [st.st_uid, st.st_gid]
    (p / 'upload-owners.json').write_text(json.dumps(owners))
    run(['tar', '--numeric-owner', '-cpf', p / 'uploads.tar', '-C', BASE / 'data', 'uploads'])
    with tarfile.open(p / 'uploads.tar') as tf:
        for m in tf:
            if '..' in Path(m.name).parts or Path(m.name).is_absolute() or not (m.isfile() or m.isdir()):
                raise ValueError('Unsafe attachment archive')
    shutil.copy2('/etc/systemd/system/simple-inventory.service', p / 'simple-inventory.service')
    for name in ['compose.yaml', '.env']:
        shutil.copy2(BASE / name, p / name)
    shutil.copy2(BASE / 'config/config.prod.yaml', p / 'docker-config.prod.yaml')
    if (BASE / 'native-config/config.prod.yaml').exists():
        shutil.copy2(BASE / 'native-config/config.prod.yaml', p / 'native-config.prod.yaml')
    if (BASE / 'current').exists():
        release = (BASE / 'current').resolve()
        (p / 'current-release.txt').write_text(str(release) + '\n')
        run(['tar', '-czf', p / 'release.tar.gz', '-C', release.parent, release.name])
    (p / 'versions.txt').write_text(versions())
    (p / 'old-image-id.txt').write_text(run(['docker', 'image', 'inspect', 'simple-inventory:http-uuid-20261008-1', '--format', '{{.Id}}'], capture_output=True, text=True).stdout)
    for f in p.iterdir():
        if f.is_file():
            f.chmod(0o600)
    files = {f.name: digest(f) for f in p.iterdir() if f.is_file()}
    manifest = {'format': 'simple-inventory-native-backup-v1', 'created_at': dt.datetime.now(dt.timezone.utc).isoformat(), 'files': files}
    (p / 'manifest.json').write_text(json.dumps(manifest, indent=2))
    (p / 'COMPLETE').write_text(digest(p / 'manifest.json') + '\n')
    verify(p)
    return p

def verify(p):
    if (p / 'COMPLETE').read_text().strip() != digest(p / 'manifest.json'):
        raise ValueError('Invalid backup completion marker')
    for name, value in json.loads((p / 'manifest.json').read_text())['files'].items():
        if digest(p / name) != value:
            raise ValueError('Backup checksum mismatch')

def restore_owners(p):
    owners = json.loads((p / 'upload-owners.json').read_text())
    root = BASE / 'data/uploads'
    for item in [root, *root.rglob('*')]:
        uid, gid = owners.get(str(item.relative_to(root)), owners['.'])
        os.chown(item, uid, gid, follow_symlinks=False)

