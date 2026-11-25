"""The seam test of #112: a nookku finding record, imported as a candidate qualification case.

TEMPORARY STAND-IN. The nookku release with the finding record is not published yet. Thus this
test installs no nookku. It reads the files of tests/nookku-seam/standin/: the finding conformance
cases of nookku at one commit, and one finding record that a local run of that commit wrote for
the library example. SOURCE.json records the commit and the SHA-256 of each file, and this test
checks each SHA-256. After the release, tests/nookku-seam/requirements.txt pins the wheel by its
SHA-256, this test runs nookku from that wheel, and the stand-in folder goes.

The example is invented. An agent answers questions about the due dates of a sample library
catalog, and gives a wrong date for one title. The finding record of that mismatch becomes a
candidate case. After the approval of a reviewer, the case fails on the old catalog snapshot and
passes on the corrected one. An unapproved candidate has no effect on qualification. The Go test
TestFindingSeam does these steps on the Host. This file gives it the records and the expected
values.

A person wrote the expected rule of each case from the README of the cases and from FINDING.md.
This test takes the input digest from hashlib, not from the Host.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
STANDIN = ROOT / 'tests/nookku-seam/standin'
CASES = STANDIN / 'conformance/finding'
# The reader result of each conformance case: None for a candidate, or the fault code and the rule.
EXPECT = {
    'valid_answer_key_mismatch': None,
    'valid_issue_row': None,
    'valid_model_issue_row': None,
    'valid_gate_record': None,
    # The reader ignores an unknown field.
    'valid_unknown_field': None,
    # The reader accepts a newer minor version.
    'valid_newer_minor_version': None,
    'missing_field': ('invalid_finding', 'missing_field'),
    'missing_key_of_an_object': ('invalid_finding', 'missing_key'),
    # The reader refuses an unknown major version.
    'unknown_major_version': ('unsupported_finding_version', 'version_major'),
    'seal_digest_mismatch': ('invalid_finding', 'seal_digest'),
    'seal_file_not_covered': ('invalid_finding', 'seal_coverage'),
    'turn_not_in_script': ('invalid_finding', 'turn_not_in_script'),
    'mismatch_with_model_author': ('invalid_finding', 'field_value'),
}
MOSS = 'When is Field Notes on Moss due?'
GARDEN = 'When is The Night Garden due?'
# The catalog that the agent of the library example read. The Go test has the same bytes.
OLD_CATALOG = '{"due": {"Field Notes on Moss": "2026-12-04", "The Night Garden": "2026-12-16"}}'


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def expectation(text, record, result):
    """The values that the Go test compares for one finding text."""
    if result is not None:
        return {'code': result[0], 'rule': result[1]}
    turn, messages, expected = record['turn'], record['script']['messages'], record['expected']
    # The case has only the keys of version 1.0. The reader ignores each other key.
    if expected is not None:
        expected = {k: expected[k] for k in ['value', 'match', 'tolerance', 'field']}
    return {'code': '', 'rule': '', 'input_digest': 'sha256:' + sha256(text.encode('utf-8')),
            'id': record['id'], 'subject': record['subject'], 'seal_digest': 'sha256:' + record['seal']['sha256'],
            'author_type': record['adjudication']['author_type'],
            'messages': messages if turn is None else messages[:turn['number']],
            'expected': expected}


class NookkuSeam(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.source = json.loads((STANDIN / 'SOURCE.json').read_text('utf-8'))
        cls.library = (STANDIN / 'library_finding.jsonl').read_text('utf-8').rstrip('\n')

    def test_standin_files(self):
        """Each stand-in file has the SHA-256 that SOURCE.json records, and no other file is there."""
        self.assertIs(self.source['temporary'], True)
        self.assertRegex(self.source['source']['commit'], r'^[0-9a-f]{40}$')
        recorded = dict(self.source['copied'])
        recorded.update({name: entry['sha256'] for name, entry in self.source['generated'].items()})
        found = {str(p.relative_to(STANDIN)): sha256(p.read_bytes()) for p in STANDIN.rglob('*')
                 if p.is_file() and p.name not in ('SOURCE.json', 'make_library_finding.py')}
        self.assertEqual(found, recorded)

    def test_expectations_agree_with_the_cases(self):
        """The cases give a problem exactly where this test expects a refusal."""
        names = {p.stem for p in CASES.glob('*.json')}
        self.assertEqual(names, set(EXPECT))
        for name in sorted(names):
            with self.subTest(case=name):
                case = json.loads((CASES / (name + '.json')).read_text('utf-8'))
                self.assertEqual(case['expect']['problem'] is None, EXPECT[name] is None)

    def test_library_record(self):
        """The library record has the values of the invented example."""
        record = json.loads(self.library)
        self.assertEqual(record['record'], 'nookku.finding')
        self.assertEqual(record['version'], '1.0')
        self.assertEqual(record['kind'], 'answer_key_mismatch')
        self.assertEqual(record['script']['messages'], [MOSS, GARDEN])
        self.assertEqual({k: record['turn'][k] for k in ['number', 'input']}, {'number': 2, 'input': GARDEN})
        self.assertEqual(json.loads(record['turn']['reply'])['due'], '2026-12-16')
        self.assertEqual(record['expected'], {'value': '2026-12-09', 'match': 'exact', 'tolerance': None, 'field': 'due'})
        self.assertEqual(record['adjudication']['author_type'], 'human')
        self.assertEqual(record['subject'], 'sha256:' + sha256(OLD_CATALOG.encode('utf-8')))

    def test_host_import(self):
        cases = []
        for name in sorted(EXPECT):
            record = json.loads((CASES / (name + '.json')).read_text('utf-8'))['record']
            text = json.dumps(record, ensure_ascii=False)
            cases.append({'name': name, 'finding': text, 'expect': expectation(text, record, EXPECT[name])})
        cases.append({'name': 'library', 'finding': self.library,
                      'expect': expectation(self.library, json.loads(self.library), None)})
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'seam-cases.json'
            path.write_text(json.dumps({'cases': cases, 'library': self.library}), 'utf-8')
            result = subprocess.run(['go', 'test', '-mod=readonly', '-count=1', '-v', '-run', '^TestFindingSeam$', '.'],
                                    cwd=ROOT / 'src/lifecycle-core', capture_output=True, text=True, timeout=300,
                                    env=os.environ | {'ENTELOS_FINDING_SEAM': str(path)})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('--- PASS: TestFindingSeam', result.stdout)
        self.assertIn('--- PASS: TestFindingSeam/library', result.stdout)


if __name__ == '__main__':
    unittest.main()
