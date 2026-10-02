#!/usr/bin/env python3
"""Initialize admin and two trusted shop operators over verified HTTPS."""
import argparse
import getpass
import json
import ssl
import urllib.parse
import urllib.request


def initialize(base, admin_old, admin_new, operators, ca=None):
    if not base.startswith('https://'):
        raise ValueError('Initialization requires HTTPS')
    context = ssl.create_default_context(cafile=ca)
    token = None

    def call(path, body=None, method=None):
        request = urllib.request.Request(base.rstrip('/') + '/api' + path,
                                         data=None if body is None else json.dumps(body).encode(),
                                         headers={'Content-Type': 'application/json', **({'Authorization': token} if token else {})},
                                         method=method or ('GET' if body is None else 'POST'))
        with urllib.request.urlopen(request, context=context, timeout=30) as response:
            value = json.load(response)
        if value['code'] != 200:
            raise ValueError('Initialization request failed: ' + path)
        return value.get('data')

    if len({o['username'] for o in operators}) != 2 or any(o['username'] == 'admin' for o in operators):
        raise ValueError('Provide two distinct operator usernames')
    for password in [admin_new, *(o['password'] for o in operators)]:
        if not 12 <= len(password) <= 50 or password == 'admin123':
            raise ValueError('Choose separate non-default passwords between 12 and 50 characters')
    if len({admin_new, *(o['password'] for o in operators)}) != 3:
        raise ValueError('Each account must have a separate password')
    token = call('/auth/login', {'username': 'admin', 'password': admin_old})['tokenValue']
    if admin_old != admin_new:
        call('/system/user/me/password', {'oldPassword': admin_old, 'newPassword': admin_new}, 'PUT')
        token = call('/auth/login', {'username': 'admin', 'password': admin_new})['tokenValue']
    roles = call('/system/role/options')
    role = next((r for r in roles if r['roleCode'] == 'SHOP_OPERATORS'), None)
    if role is None:
        call('/system/role', {'roleName': '店铺经营者', 'roleCode': 'SHOP_OPERATORS', 'status': 1})
        role = next(r for r in call('/system/role/options') if r['roleCode'] == 'SHOP_OPERATORS')

    def flatten(nodes):
        return [n for node in nodes for n in [node, *flatten(node.get('children') or [])]]

    menus = [m['id'] for m in flatten(call('/system/menu/tree')) if (m.get('path') or '').startswith('/business/')]
    if not menus:
        raise ValueError('Business menus are missing; check explicit seed migrations')
    call(f'/system/role/{role["id"]}/menus', {'menuIds': menus}, 'PUT')
    admin_token = token
    for operator in operators:
        token = admin_token
        # Existing users are refused: this initializer never resets another person's password.
        existing = call('/system/user/page?' + urllib.parse.urlencode({'username': operator['username']}))['records']
        if any(u['username'] == operator['username'] for u in existing):
            raise ValueError('Operator already exists; inspect it manually instead of overwriting')
        call('/system/user', {'username': operator['username'], 'nickname': operator['nickname'], 'deptId': 1, 'status': 1})
        user = next(u for u in call('/system/user/page?' + urllib.parse.urlencode({'username': operator['username']}))['records'] if u['username'] == operator['username'])
        call(f'/system/user/{user["id"]}/roles', {'roleIds': [role['id']]}, 'PUT')
        token = call('/auth/login', {'username': operator['username'], 'password': 'admin123'})['tokenValue']
        call('/system/user/me/password', {'oldPassword': 'admin123', 'newPassword': operator['password']}, 'PUT')
        token = call('/auth/login', {'username': operator['username'], 'password': operator['password']})['tokenValue']
        print('Initialized operator: ' + operator['username'])
    print('Account initialization complete; credentials are not written to disk')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--ca', help='Explicit isolated/local CA; omitted for a public certificate')
    args = parser.parse_args()
    admin_old = getpass.getpass('Current administrator password: ')
    admin_new = getpass.getpass('New administrator password (12–50 characters): ')
    operators = []
    for n in (1, 2):
        operators.append({'username': input(f'Operator {n} username: ').strip(),
                          'nickname': input(f'Operator {n} display name: ').strip(),
                          'password': getpass.getpass(f'Operator {n} password (12–50 characters): ')})
    initialize(args.url, admin_old, admin_new, operators, args.ca)
