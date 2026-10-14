#!/usr/bin/env python3
"""Render private regional runtime files offline; never deploy or enable admission."""
import argparse
import base64
import copy
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
from urllib.parse import urlsplit


def require(ok, message):
    if not ok:
        raise ValueError(message)


def fields(obj, names):
    require(isinstance(obj, dict) and set(obj) == set(names.split()), 'unexpected or missing fields')


def identifier(value):
    require(isinstance(value, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,62}', value), 'invalid identifier')


def validate(spec, secrets):
    fields(spec, 'epoch legacy_redis_user primary_instances cutover regions')
    fields(secrets, 'regions')
    identifier(spec['epoch'])
    identifier(spec['legacy_redis_user'])
    primary = spec['primary_instances']
    require(isinstance(primary, list) and primary, 'primary inventory required')
    for instance in primary:
        identifier(instance)
    require(len(set(primary)) == len(primary), 'duplicate primary instance')
    cutover = spec['cutover']
    fields(cutover, 'endpoint namespace workloads')
    endpoint = urlsplit(cutover['endpoint'])
    require(endpoint.scheme == 'https' and endpoint.hostname and endpoint.username is None and endpoint.password is None and not endpoint.query and not endpoint.fragment and not endpoint.path, 'cutover requires an HTTPS API origin')
    identifier(cutover['namespace'])
    require(isinstance(cutover['workloads'], dict) and cutover['workloads'], 'StatefulSet UID inventory required')
    expected = set()
    for name, workload in cutover['workloads'].items():
        identifier(name)
        fields(workload, 'uid replicas')
        require(isinstance(workload['uid'], str) and re.fullmatch(r'[a-zA-Z0-9-]{1,128}', workload['uid']), 'invalid workload UID')
        require(type(workload['replicas']) is int and 1 <= workload['replicas'] <= 1000, 'invalid replica inventory')
        expected.update(f'{name}-{ordinal}' for ordinal in range(workload['replicas']))
    require(expected == set(primary), 'primary names must exactly match StatefulSet ordinal inventory')
    require(isinstance(spec['regions'], list) and spec['regions'], 'regions required')
    regions, assigned, instances = set(), set(), set(primary)
    for region in spec['regions']:
        fields(region, 'id primary instance_ids endpoint primary_pin regional_pin account_ids user_ids api_key_ids group_id slots rpm_units lifetime_seconds')
        identifier(region['id'])
        require(region['id'] not in regions, 'duplicate region')
        regions.add(region['id'])
        require(region['primary'] in primary and region['primary'] not in assigned, 'one publisher per primary process required')
        assigned.add(region['primary'])
        require(isinstance(region['instance_ids'], list) and region['instance_ids'], 'regional instances required')
        for instance in region['instance_ids']:
            identifier(instance)
            require(instance not in instances, 'duplicate instance identity')
            instances.add(instance)
        endpoint = urlsplit(region['endpoint'])
        require(endpoint.scheme == 'https' and endpoint.hostname and endpoint.username is None and endpoint.password is None and not endpoint.query and not endpoint.fragment and endpoint.path in ('', '/') and endpoint.port is None, 'HTTPS origin required without port or credentials')
        for name in ('primary_pin', 'regional_pin'):
            require(isinstance(region[name], str) and re.fullmatch(r'[0-9a-f]{64}', region[name]), 'invalid certificate pin')
        for name in ('account_ids', 'user_ids', 'api_key_ids'):
            values = region[name]
            require(isinstance(values, list) and values and all(type(v) is int and v > 0 for v in values), 'positive IDs required')
            require(len(set(values)) == len(values), 'duplicate IDs')
        require(type(region['group_id']) is int and region['group_id'] > 0, 'group ID required')
        require(type(region['slots']) is int and 1 <= region['slots'] <= 10000, 'invalid slot capacity')
        require(type(region['rpm_units']) is int and 1 <= region['rpm_units'] <= 1000000, 'invalid RPM budget')
        require(type(region['lifetime_seconds']) is int and 30 <= region['lifetime_seconds'] <= 3600, 'invalid lifetime')
        material = secrets['regions'][region['id']]
        fields(material, 'key_id encryption_key auth_lookup_key')
        identifier(material['key_id'])
        for name in ('encryption_key', 'auth_lookup_key'):
            require(len(base64.b64decode(material[name], validate=True)) == 32, '32-byte key required')
    require(assigned == set(primary), 'every primary must have one regional publisher')
    require(set(secrets['regions']) == regions, 'secret region assignment mismatch')


def documents(spec, secrets):
    validate(spec, secrets)
    files = {}
    for region in spec['regions']:
        material = secrets['regions'][region['id']]
        for instance in [region['primary']] + region['instance_ids']:
            primary = instance == region['primary']
            tls = '/run/sub2api-regional/tls/'
            base = dict(mode='publisher' if primary else 'subscriber', policy=dict(region_id=region['id'], max_snapshot_age_seconds=60, max_clock_skew_seconds=2), keys={material['key_id']: material['encryption_key']}, key_id=material['key_id'], bootstrap=False, ca_file=tls+'ca.pem', cert_file=tls+'cert.pem', key_file=tls+'key.pem', trusted_client_cert_sha256=[region['regional_pin'] if primary else region['primary_pin']], trusted_server_cert_sha256=region['primary_pin'], interval_seconds=5, lifetime_seconds=30)
            for kind, port in [('scheduler', 9443), ('auth', 9444)]:
                stream = copy.deepcopy(base)
                stream.update(snapshot_kind=kind, listen=f'0.0.0.0:{port}', endpoint='' if primary else region['endpoint'].rstrip('/')+f':{port}/internal/regional/{kind}', state_file='' if primary else f'/app/data/regional/{instance}/{kind}.json', allowed_account_ids=region['account_ids'] if kind == 'scheduler' else [], buckets=[dict(GroupID=region['group_id'], Platform='openai', Mode='single')] if kind == 'scheduler' else [])
                if kind == 'auth':
                    stream.update(allowed_user_ids=region['user_ids'], auth_lookup_key=material['auth_lookup_key'])
                files[f'{instance}/{kind}.json'] = stream
            concurrency = dict(Mode='primary' if primary else 'regional', Region=region['id'], Instance=instance, Epoch=spec['epoch'], PrimaryInstances=spec['primary_instances'] if primary else [], LegacyRedisUser=spec['legacy_redis_user'] if primary else '', Listen='0.0.0.0:9445' if primary else '', Endpoint='' if primary else region['endpoint'].rstrip('/')+':9445', CAFile=tls+'ca.pem', CertFile=tls+'cert.pem', KeyFile=tls+'key.pem', ServerPin=region['primary_pin'], LifetimeSeconds=region['lifetime_seconds'])
            if primary:
                concurrency['Scope'] = dict(Region=region['id'], CertificateSHA256=region['regional_pin'], UserIDs=region['user_ids'], AccountIDs=region['account_ids'], APIKeyIDs=region['api_key_ids'], MaximumGrant='1000000000', MaximumSlots=region['slots'], MaximumRPM=region['rpm_units'], MaximumLifetime=region['lifetime_seconds']*1000000000)
                cutover = spec['cutover']
                concurrency['Cutover'] = dict(Endpoint=cutover['endpoint'], Namespace=cutover['namespace'], Workloads={name: value['uid'] for name, value in cutover['workloads'].items()}, CAFile='/run/sub2api-inventory/ca.crt', TokenFile='/run/sub2api-inventory/token')
            else:
                concurrency['Targets'] = [dict(Resource=dict(kind=kind, resource_id=value), Capacity=region['slots']) for kind, values in [('account', region['account_ids']), ('user', region['user_ids']), ('api_key', region['api_key_ids'])] for value in values]
                concurrency['RPMTargets'] = [dict(AccountID=value, Units=region['rpm_units']) for value in region['account_ids']]
            files[f'{instance}/concurrency.json'] = concurrency
            files[f'{instance}/runtime.json'] = dict(runtime=dict(role='full' if primary else 'gateway', regional_auth_file='/run/sub2api-regional/auth.json', regional_scheduler_file='/run/sub2api-regional/scheduler.json', regional_concurrency_file='/run/sub2api-regional/concurrency.json'))
    return files


def render(spec, secrets, destination):
    files = documents(spec, secrets)
    destination = Path(destination)
    require(not destination.exists(), 'refusing to replace runtime identity')
    require(destination.parent.is_dir(), 'output parent must exist')
    temporary = Path(tempfile.mkdtemp(prefix='.regional-render-', dir=destination.parent))
    try:
        for relative, value in files.items():
            target = temporary / relative
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w') as output:
                json.dump(value, output, indent=2)
                output.write('\n')
        temporary.rename(destination)
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)
    return len(files)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--topology', required=True, type=Path)
    parser.add_argument('--secrets', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    try:
        require(args.secrets.stat().st_mode & 0o077 == 0, 'secrets must have private permissions')
        count = render(json.loads(args.topology.read_text()), json.loads(args.secrets.read_text()), args.output)
    except (ValueError, OSError, TypeError, KeyError):
        parser.exit(2, 'Invalid regional configuration or output path; no deployment performed.\n')
    print(f'Rendered {count} private files. Admission remains disabled; no deployment performed.')


if __name__ == '__main__':
    main()
