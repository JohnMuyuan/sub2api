import base64
import copy
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest

import render


class RenderTests(unittest.TestCase):
    def setUp(self):
        self.spec = json.loads(Path(__file__).with_name('topology.example.json').read_text())
        self.secrets = {'regions': {
            r['id']: {'key_id': 'test',
                      'encryption_key': base64.b64encode(bytes([index + 1]) * 32).decode(),
                      'auth_lookup_key': base64.b64encode(bytes([index + 10]) * 32).decode()}
            for index, r in enumerate(self.spec['regions'])}}

    def test_two_regions_share_accounts_but_not_snapshot_identity(self):
        files = render.documents(self.spec, self.secrets)
        self.assertEqual(len(files), 24)
        states = set()
        for region in self.spec['regions']:
            primary = files[region['primary'] + '/concurrency.json']
            self.assertEqual(primary['PrimaryInstances'], ['primary-a-0', 'primary-b-0'])
            self.assertEqual(primary['Scope']['AccountIDs'], [101])
            self.assertEqual(primary['Scope']['MaximumLifetime'], 60_000_000_000)
            for instance in region['instance_ids']:
                for stream in ('auth', 'scheduler'):
                    doc = files[f'{instance}/{stream}.json']
                    self.assertFalse(doc['bootstrap'])
                    self.assertTrue(doc['endpoint'].endswith('/internal/regional/' + stream))
                    self.assertNotIn(doc['state_file'], states)
                    states.add(doc['state_file'])
                self.assertEqual(files[f'{instance}/scheduler.json']['allowed_account_ids'], [101])
                self.assertEqual(files[f'{instance}/auth.json']['allowed_user_ids'], [201])
                self.assertEqual(files[f'{instance}/runtime.json']['runtime']['role'], 'gateway')

    def test_invalid_topology_cannot_write_partial_output(self):
        changes = [
            lambda s: s['regions'][0].update(id='../escape'),
            lambda s: s['regions'][0].update(instance_ids=['primary-a-0']),
            lambda s: s['regions'][1].update(primary='primary-a-0'),
            lambda s: s['regions'][0].update(endpoint='https://user:secret@example.invalid'),
            lambda s: s['regions'][0].update(endpoint='http://example.invalid'),
            lambda s: s['regions'][0].update(endpoint='https://example.invalid:9443'),
            lambda s: s['regions'][0].update(account_ids=[101, 101]),
            lambda s: s['regions'][0].update(user_ids=[True]),
            lambda s: s['regions'][0].update(primary_pin='x' * 64),
            lambda s: s['regions'][0].update(lifetime_seconds=3601),
            lambda s: s['regions'][0].update(unknown=True),
        ]
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'rendered'
            for change in changes:
                with self.subTest(change=change):
                    spec = copy.deepcopy(self.spec)
                    change(spec)
                    with self.assertRaises(ValueError):
                        render.render(spec, self.secrets, target)
                    self.assertFalse(target.exists())
                    self.assertEqual(list(Path(directory).iterdir()), [])

    def test_private_permissions_and_existing_identity_survives(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'rendered'
            old_umask = os.umask(0)
            try:
                self.assertEqual(render.render(self.spec, self.secrets, target), 24)
            finally:
                os.umask(old_umask)
            before = {p.relative_to(target): p.read_bytes() for p in target.rglob('*.json')}
            for item in [target] + list(target.rglob('*')):
                self.assertEqual(stat.S_IMODE(item.stat().st_mode), 0o700 if item.is_dir() else 0o600)
            with self.assertRaises(ValueError):
                render.render(self.spec, self.secrets, target)
            self.assertEqual(before, {p.relative_to(target): p.read_bytes() for p in target.rglob('*.json')})

    def test_cli_rejects_public_secret_file_without_leaking(self):
        with tempfile.TemporaryDirectory() as directory:
            secrets = Path(directory) / 'secrets.json'
            secrets.write_text(json.dumps(self.secrets))
            secrets.chmod(0o644)
            result = subprocess.run([
                sys.executable, str(Path(__file__).with_name('render.py')),
                '--topology', str(Path(__file__).with_name('topology.example.json')),
                '--secrets', str(secrets), '--output', str(Path(directory) / 'out'),
            ], capture_output=True, text=True, check=False)
            self.assertEqual(result.returncode, 2)
            self.assertNotIn('Traceback', result.stderr)
            self.assertNotIn(self.secrets['regions']['region-a']['encryption_key'], result.stderr)
            self.assertFalse((Path(directory) / 'out').exists())


if __name__ == '__main__':
    unittest.main()
