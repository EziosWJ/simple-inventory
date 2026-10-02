#!/usr/bin/env python3
"""Single-host deployment, coherent full backups and isolated restores (Python 3)."""
import argparse
import datetime as dt
import fcntl
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess as sp
import sys
import tarfile
import uuid

PACKAGE = Path(__file__).resolve().parent
FORMAT = 'simple-inventory-full-backup-v1'


def run(args, **kwargs):
    return sp.run(args, check=True, **kwargs)


class Deployment:
    def __init__(self, env, directory=None):
        self.env = Path(env).resolve(strict=True)
        self.directory = Path(directory or (self.env.parent if (self.env.parent / 'compose.yaml').is_file() else PACKAGE)).resolve()
        if stat.S_IMODE(self.env.stat().st_mode) & 0o077:
            raise ValueError('Private env must have mode 600 or stricter')
        self.command = ['docker', 'compose', '--profile', 'tools', '--env-file', str(self.env), '-f', str(self.directory / 'compose.yaml')]
        override = self.directory / 'images.override.yaml'
        if override.exists():
            self.command += ['-f', str(override)]
        self.config = json.loads(self.call('config', '--format', 'json', capture_output=True).stdout)
        self.project = self.config['name']
        if not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', self.project):
            raise ValueError('Invalid deployment project name')
        app = self.config['services']['api']['environment']
        if any('replace-with-' in str(v) for v in app.values()):
            raise ValueError('Replace placeholder credentials before starting')
        if len(app['APP_JWT__SECRET']) < 32 or len(app['APP_DATABASE__PASSWORD']) < 20:
            raise ValueError('Use a random JWT secret of at least 32 characters and DB password of at least 20 characters')

    def call(self, *args, **kwargs):
        return run(self.command + list(args), **kwargs)

    def sql(self, query):
        return self.call('exec', '-T', 'db', 'sh', '-ec',
                         'psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At',
                         input=query.encode(), capture_output=True).stdout.decode().strip()

    def volume(self, name):
        return self.config['volumes'][name]['name']

    def images(self):
        result = {}
        for name in ('api', 'migrate', 'db', 'proxy'):
            tag = self.config['services'][name]['image']
            image_id = run(['docker', 'image', 'inspect', tag, '--format', '{{.Id}}'], capture_output=True).stdout.decode().strip()
            result[name] = {'tag': tag, 'id': image_id}
        return result

    def check(self):
        self.call('run', '--rm', '--no-deps', 'migrate', 'check', '--kind', 'all')

    def start(self, migrate=False):
        self.call('up', '-d', '--wait', 'db')
        if migrate:
            # Stop all application writers before an explicit upgrade.
            self.call('stop', 'proxy', 'api')
            self.call('run', '--rm', 'migrate', 'up', '--kind', 'all')
        self.check()
        self.call('up', '-d', '--wait', 'api', 'proxy')


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def verify(directory):
    directory = Path(directory).resolve(strict=True)
    if not (directory / 'COMPLETE').is_file():
        raise ValueError('Incomplete backup: no COMPLETE marker')
    manifest = json.loads((directory / 'manifest.json').read_text())
    if manifest.get('format') != FORMAT:
        raise ValueError('Unknown backup format')
    required = {'database.dump', 'uploads.tar', 'deployment.env', 'compose.yaml', 'Caddyfile', 'images.json', 'versions.json'}
    if set(manifest.get('files', {})) != required:
        raise ValueError('Backup is missing required files')
    if (directory / 'COMPLETE').read_text().strip() != digest(directory / 'manifest.json'):
        raise ValueError('Manifest checksum mismatch')
    for name, checksum in manifest['files'].items():
        path = directory / name
        if path.is_symlink() or not path.is_file() or digest(path) != checksum:
            raise ValueError('Backup checksum mismatch: ' + name)
    with tarfile.open(directory / 'uploads.tar') as archive:
        for member in archive:
            path = Path(member.name)
            if path.is_absolute() or '..' in path.parts or not (member.isfile() or member.isdir()):
                raise ValueError('Unsafe upload archive entry')
    return manifest


def backup(deployment, root):
    root = Path(root).resolve()
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    if stat.S_IMODE(root.stat().st_mode) & 0o077:
        raise ValueError('Backup directory must have mode 700')
    with (root / '.backup.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError('Another backup is running; try later') from None
        now = dt.datetime.now(dt.timezone.utc)
        name = f'inventory-{deployment.project}-{now:%Y%m%dT%H%M%SZ}-{uuid.uuid4().hex[:8]}'
        partial = root / ('.partial-' + name)
        partial.mkdir(mode=0o700)
        running = set(deployment.call('ps', '--services', '--status', 'running', capture_output=True).stdout.decode().split()) & {'api', 'proxy'}
        images = deployment.images()
        # Fail before maintenance if an edited .env points at a different release.
        # A backup must describe the actual deployment, not its intended upgrade.
        for service in ('api', 'db', 'proxy'):
            container = deployment.call('ps', '-a', '-q', service, capture_output=True).stdout.decode().strip()
            if not container:
                raise ValueError('Backup requires an initialized deployment: ' + service)
            actual = run(['docker', 'inspect', container, '--format', '{{.Image}}'], capture_output=True).stdout.decode().strip()
            if actual != images[service]['id']:
                raise ValueError('Configuration and running release differ; back up before changing release: ' + service)
        deployment.check()
        try:
            # stop waits for API termination; no live app process can modify DB or files.
            deployment.call('stop', 'proxy', 'api')
            with (partial / 'database.dump').open('wb') as output:
                deployment.call('exec', '-T', 'db', 'sh', '-ec',
                                'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner --no-acl', stdout=output)
            with (partial / 'database.dump').open('rb') as source:
                deployment.call('exec', '-T', 'db', 'pg_restore', '--list', stdin=source, stdout=sp.DEVNULL)
            with (partial / 'uploads.tar').open('wb') as output:
                run(['docker', 'run', '--rm', '--network', 'none', '--user', '0', '--entrypoint', '/bin/tar',
                     '--mount', f'type=volume,src={deployment.volume("uploads")},dst=/source,readonly',
                     images['api']['id'], '-C', '/source', '-cf', '-', '.'], stdout=output)
            shutil.copyfile(deployment.env, partial / 'deployment.env')
            shutil.copyfile(deployment.directory / 'compose.yaml', partial / 'compose.yaml')
            # Capture the actual mounted Caddyfile, including an isolated test configuration.
            caddy = next(v['source'] for v in deployment.config['services']['proxy']['volumes'] if v['target'] == '/etc/caddy/Caddyfile')
            shutil.copyfile(caddy, partial / 'Caddyfile')
            (partial / 'images.json').write_text(json.dumps(images, indent=2))
            versions = {kind: deployment.sql(f'SELECT version_id, is_applied FROM goose_{kind}_db_version ORDER BY id;') for kind in ('schema', 'seed')}
            (partial / 'versions.json').write_text(json.dumps(versions, indent=2))
            files = {p.name: digest(p) for p in partial.iterdir()}
            manifest = {'format': FORMAT, 'project': deployment.project, 'created_at': now.isoformat(),
                        'release': deployment.config['services']['api'].get('labels', {}).get('org.opencontainers.image.version'), 'files': files}
            (partial / 'manifest.json').write_text(json.dumps(manifest, indent=2))
            (partial / 'COMPLETE').write_text(digest(partial / 'manifest.json') + '\n')
            verify(partial)
            os.rename(partial, root / name)
        finally:
            # Also runs on dump, filesystem, checksum or other failure. No success marker is exposed in root.
            if running:
                deployment.call('up', '-d', '--wait', *sorted(running))
        # Prune only verified successful backups of this project, always keep the latest.
        cutoff = now - dt.timedelta(days=30)
        for candidate in root.iterdir():
            if candidate.is_symlink() or not candidate.is_dir() or not candidate.name.startswith('inventory-' + deployment.project + '-') or candidate.name == name:
                continue
            try:
                previous = verify(candidate)
                if previous['project'] == deployment.project and dt.datetime.fromisoformat(previous['created_at']) < cutoff:
                    shutil.rmtree(candidate)
            except (ValueError, KeyError, OSError, json.JSONDecodeError, tarfile.TarError):
                continue
        print('BACKUP_COMPLETE ' + str(root / name), flush=True)


def restore(args):
    archive = Path(args.backup).resolve(strict=True)
    manifest = verify(archive)
    if args.project == manifest['project'] or not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', args.project):
        raise ValueError('Restore requires a distinct valid project name')
    existing = run(['docker', 'volume', 'ls', '-q', '--filter', 'label=com.docker.compose.project=' + args.project], capture_output=True).stdout
    containers = run(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=' + args.project], capture_output=True).stdout
    if existing.strip() or containers.strip():
        raise ValueError('Restore project already has containers or volumes; original data is never overwritten')
    target = Path(args.target).resolve()
    if target.exists():
        raise ValueError('Restore target must be a new directory')
    images = json.loads((archive / 'images.json').read_text())
    for image in images.values():
        run(['docker', 'image', 'inspect', image['id']], stdout=sp.DEVNULL)
    target.mkdir(mode=0o700, parents=True)
    for name in ('compose.yaml', 'Caddyfile'):
        shutil.copyfile(archive / name, target / name)
    archived_config = json.loads(run(['docker', 'compose', '--profile', 'tools', '--env-file', str(archive / 'deployment.env'), '-f', str(archive / 'compose.yaml'), 'config', '--format', 'json'], capture_output=True).stdout)
    domain = archived_config['services']['proxy']['environment']['DOMAIN']
    env = (archive / 'deployment.env').read_text()
    network = ipaddress.ip_network(args.subnet)
    proxy = ipaddress.ip_address(args.proxy_ip)
    if proxy not in network or proxy in (network[2], network[4]):
        raise ValueError('Proxy IP must be inside the subnet and distinct from DB/API addresses')
    changes = {'INVENTORY_PROJECT': args.project, 'HTTP_PORT': str(args.http_port), 'HTTPS_PORT': str(args.https_port),
               'PUBLIC_BIND': '127.0.0.1', 'BACKEND_SUBNET': args.subnet, 'PROXY_IP': args.proxy_ip,
               'PUBLIC_ORIGIN': 'https://' + domain + ('' if args.https_port == 443 else ':' + str(args.https_port)),
               'DB_IP': str(network[2]), 'API_IP': str(network[4]), 'CADDYFILE': str(target / 'Caddyfile')}
    for key, value in changes.items():
        env = re.sub(r'^' + key + r'=.*\n?', '', env, flags=re.M) + f'{key}={value}\n'
    private = target / 'deployment.env'
    private.write_text(env)
    # JSON is valid YAML; use exact old IDs even if release tags have moved locally.
    (target / 'images.override.yaml').write_text(json.dumps({'services': {name: {'image': image['id']} for name, image in images.items()}}))
    deployment = Deployment(private, target)
    deployment.call('up', '-d', '--wait', 'db')
    with (archive / 'database.dump').open('rb') as source:
        deployment.call('exec', '-T', 'db', 'sh', '-ec',
                        'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --exit-on-error --no-owner --no-acl', stdin=source)
    versions = json.loads((archive / 'versions.json').read_text())
    for kind, expected in versions.items():
        if deployment.sql(f'SELECT version_id, is_applied FROM goose_{kind}_db_version ORDER BY id;') != expected:
            raise ValueError('Restored migration history does not match backup')
    # Create a fresh upload volume without ever starting the application writer.
    deployment.call('create', 'api')
    with (archive / 'uploads.tar').open('rb') as source:
        run(['docker', 'run', '--rm', '-i', '--network', 'none', '--user', '0', '--entrypoint', '/bin/tar',
             '--mount', f'type=volume,src={deployment.volume("uploads")},dst=/restore', images['api']['id'],
             '-C', '/restore', '-xpf', '-'], stdin=source)
    deployment.start()
    print('RESTORE_COMPLETE ' + str(target), flush=True)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--env', default=str(PACKAGE / '.env'))
    commands = parser.add_subparsers(dest='action', required=True)
    commands.add_parser('start', help='Read-only version check and start; never migrate')
    commands.add_parser('migrate-start', help='Stop writers, explicitly migrate, check and start')
    b = commands.add_parser('backup'); b.add_argument('--directory', required=True)
    v = commands.add_parser('verify'); v.add_argument('backup')
    r = commands.add_parser('restore'); r.add_argument('backup'); r.add_argument('--target', required=True); r.add_argument('--project', required=True)
    r.add_argument('--http-port', type=int, required=True); r.add_argument('--https-port', type=int, required=True)
    r.add_argument('--subnet', required=True); r.add_argument('--proxy-ip', required=True)
    args = parser.parse_args()
    if args.action == 'verify':
        verify(args.backup); print('BACKUP_VERIFIED')
    elif args.action == 'restore':
        restore(args)
    else:
        deployment = Deployment(args.env)
        if args.action == 'backup': backup(deployment, args.directory)
        else: deployment.start(migrate=args.action == 'migrate-start')


if __name__ == '__main__':
    try:
        main()
    except sp.CalledProcessError as error:
        print(f'FAILED: external command returned {error.returncode}; inspect deployment logs', file=sys.stderr)
        sys.exit(1)
    except (ValueError, OSError, KeyError, tarfile.TarError) as error:
        print('FAILED: ' + str(error), file=sys.stderr)
        sys.exit(1)
