#!/usr/bin/env python3
import datetime as dt
import os
from pathlib import Path
import tarfile
import tempfile
import yaml
from native_common import BASE, DB, dbcmd, digest, run, verify

backup = Path((BASE / 'native-tools/initial-backup.txt').read_text().strip())
verify(backup)
scratch = 'inventory_restore_' + dt.datetime.now(dt.timezone.utc).strftime('%Y%m%d%H%M%S')
created = False
try:
    run(dbcmd('createdb -U "$POSTGRES_USER" -O jxc "$1"', scratch))
    created = True
    with (backup / 'database.dump').open('rb') as f:
        run(dbcmd('pg_restore -U "$POSTGRES_USER" --role=jxc -d "$1" --no-owner --no-acl --exit-on-error', scratch), stdin=f)
    release = (BASE / 'current').resolve()
    cfg = yaml.safe_load((BASE / 'native-config/config.prod.yaml').read_text())
    env = {**os.environ, 'APP_ENV': 'prod', 'APP_DATABASE__URL': cfg['database']['url'].replace('/jxc?', '/' + scratch + '?')}
    if env['APP_DATABASE__URL'] == cfg['database']['url']:
        raise ValueError('Scratch database URL unchanged')
    run([release / 'simple-inventory-migrate', 'check', '--kind', 'all'], cwd=release, env=env)
    q = 'SELECT version_id,is_applied FROM goose_schema_db_version ORDER BY id; SELECT version_id,is_applied FROM goose_seed_db_version ORDER BY id;'
    restored = run(dbcmd('psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -At', scratch), input=q.encode(), capture_output=True).stdout.decode()
    if restored != (backup / 'versions.txt').read_text():
        raise ValueError('Restored migration versions differ')
    with tempfile.TemporaryDirectory(prefix='inventory-restore-') as tmp:
        run(['tar', '-xpf', backup / 'uploads.tar', '-C', tmp])
        count = 0
        for f in (BASE / 'data/uploads').rglob('*'):
            if f.is_file():
                other = Path(tmp) / 'uploads' / f.relative_to(BASE / 'data/uploads')
                if digest(f) != digest(other):
                    raise ValueError('Restored attachment differs')
                count += 1
    print('RESTORE_VERIFIED scratch_database=' + scratch + ' attachments=' + str(count), flush=True)
finally:
    if created:
        run(dbcmd('dropdb -U "$POSTGRES_USER" "$1"', scratch))
