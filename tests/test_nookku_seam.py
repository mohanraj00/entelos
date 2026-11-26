"""The seam test of #112: a nookku finding record, imported as a candidate qualification case.

This test runs the nookku release that tests/nookku-seam/requirements.txt pins by the SHA-256 of
its wheel. verify.py installs that wheel in a virtual environment and runs this file there.
The test has no copy of a nookku file, and it reads no nookku source.

The example is invented. An agent answers questions about the due dates of a sample library
catalog, and gives a wrong date for one title. nookku runs the script, a person adjudicates the
mismatch as agent_wrong, and nookku exports the finding records: one mismatch, one issue row of
a person, and one mismatch of a gate run. The mismatch record becomes a candidate case. After the
approval of a reviewer, the case fails on the old catalog snapshot and passes on the corrected
one. An unapproved candidate has no effect on qualification. The Go test TestFindingSeam does
these steps on the Host. This file gives it the records and the expected values.

The agent is a local process with fixed replies. No model runs, and the run calls no service.

A person wrote each changed record and its expected rule from FINDING.md. The reader of the
release must agree on which records it refuses. This test takes the input digest from hashlib,
not from the Host.
"""
import contextlib
import hashlib
import importlib.metadata
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

from nookku import bridge, finding_record
from nookku.cli import main

ROOT = Path(__file__).resolve().parents[1]
VERSION = '0.6.1'
MOSS = 'When is Field Notes on Moss due?'
GARDEN = 'When is The Night Garden due?'
# The catalog that the agent reads. The due date of The Night Garden is wrong. The Go test has
# the same bytes.
OLD_CATALOG = '{"due": {"Field Notes on Moss": "2026-12-04", "The Night Garden": "2026-12-16"}}'
# The loan list of the library, as an answer key. A person wrote these dates.
KEY = {'source': 'the loan list of the sample library', 'version': '1',
       'answers': [{'question': MOSS, 'answer': '2026-12-04'}, {'question': GARDEN, 'answer': '2026-12-09'}]}
SPEC = '''# Sample library

## Answer key

| File | Field | Match |
|---|---|---|
| `keys/due.json` | `due` | exact |
'''
EGRESS = {'model_apis': [], 'backends': [], 'mcp_servers': [], 'databases': [], 'other': []}
GATE = {'v': 1, 'tests': [{'name': 'due-dates', 'script': 'script.jsonl', 'answer_key': 'keys/due.json'}]}
REASON = 'The loan list gives 2026-12-09.'
ISSUE = 'The reply gives a due date that the loan list does not have.'
# The agent speaks the agent contract of nookku: one JSON line in, one JSON line out.
AGENT = '''
import json, sys
due = json.load(open(sys.argv[1]))["due"]
for raw in sys.stdin.buffer:
    request = json.loads(raw)
    title = next((t for t in sorted(due) if t in request["message"]), None)
    reply = {"reply": "I do not know that title."} if title is None else {
        "due": due[title], "reply": title + " is due on " + due[title] + "."}
    sys.stdout.write(json.dumps({"v": 1, "id": request["id"], "reply": json.dumps(reply)}) + "\\n")
    sys.stdout.flush()
'''
OTHER_SHA = 'ab' * 32


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def cli(*words):
    """Run one nookku command in this process. Give the exit code, the output and the messages."""
    # The export writes bytes, so the standard output of a command has a buffer.
    raw, err = io.BytesIO(), io.StringIO()
    out = io.TextIOWrapper(raw, encoding='utf-8', newline='')
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = main([str(w) for w in words])
    out.flush()
    return code, raw.getvalue().decode('utf-8'), err.getvalue()


def register(root):
    # The agent calls no service, so each list of the egress answer is empty. A start uses the
    # answer one time.
    answer = root / '.nookku' / 'egress.json'
    answer.write_text(json.dumps(EGRESS), 'utf-8')
    code, out, err = cli('register', '--root', root, '--file', answer)
    assert code == 0, out + err


def adjudicate(root, test):
    code, out, err = cli('adjudicate', test, '--root', root, '--author', 'Asha', '--turn', '2',
                         '--outcome', 'agent_wrong', '--reason', REASON)
    assert code == 0, out + err


def export(work):
    """Run the library test in a new project folder. Give the exported finding lines by name."""
    root = work / 'library'
    (root / '.nookku').mkdir(parents=True)
    (root / 'keys').mkdir()
    (work / 'agent.py').write_text(AGENT, 'utf-8')
    (work / 'catalog.json').write_text(OLD_CATALOG, 'utf-8')
    (root / 'APP_SPEC.md').write_text(SPEC, 'utf-8')
    (root / 'keys' / 'due.json').write_text(json.dumps(KEY), 'utf-8')
    config = {'entry': [sys.executable, str(work / 'agent.py'), str(work / 'catalog.json')], 'models': [],
              'spec': ['APP_SPEC.md'], 'page': False}
    (root / '.nookku' / 'config.json').write_text(json.dumps(config), 'utf-8')
    (root / 'script.jsonl').write_text(''.join(json.dumps({'said': s}) + '\n' for s in [MOSS, GARDEN]), 'utf-8')
    (root / 'gate.json').write_text(json.dumps(GATE), 'utf-8')
    subject = 'sha256:' + sha256(OLD_CATALOG.encode('utf-8'))

    # One test: a mismatch that a person adjudicates, and one issue row of a person.
    register(root)
    code, out, err = cli('run', '--script', root / 'script.jsonl', '--root', root, '--json')
    folder = bridge.latest_test(root)
    assert folder is not None, out + err
    adjudicate(root, folder.name)
    code, out, err = cli('evaluation', 'add', folder.name, '--root', root, '--class', 'unsupported_reply',
                         '--turn', '2', '--evidence', 'turn 2', '--text', ISSUE, '--author', 'Asha')
    assert code == 0, out + err
    code, out, err = cli('finding', 'export', folder.name, '--root', root, '--subject', subject)
    assert code == 0, out + err
    library, issue = out.splitlines()

    # One gate run with the same script. The gate fails, and a person adjudicates its mismatch.
    register(root)
    code, out, err = cli('gate', 'run', root / 'gate.json', '--subject', subject, '--root', root, '--json')
    result = json.loads(out)['result']
    adjudicate(root, result['tests'][0]['test'])
    code, out, err = cli('finding', 'export', '--gate', result['gate'], '--root', root)
    assert code == 0, out + err
    (gate,) = out.splitlines()
    return {'library': library, 'issue_row': issue, 'gate_record': gate}


# Each change that a person makes to the library record, and the reader result that FINDING.md
# gives: None for a candidate, or the fault code and the rule.
def unknown_field(r):
    r['shelf'] = 'B4'
    r['expected']['unit'] = 'date'


def newer_minor_version(r):
    r['version'] = '1.7'
    r['renewals'] = 2


def missing_field(r):
    del r['subject']


def missing_key_of_an_object(r):
    del r['script']['sha256']


def unknown_major_version(r):
    r['version'] = '2.0'


def seal_digest_mismatch(r):
    r['seal']['sha256'] = OTHER_SHA


def seal_file_not_covered(r):
    r['seal']['files']['findings.json'] = OTHER_SHA


def turn_not_in_script(r):
    r['turn']['input'] = MOSS


def mismatch_with_model_author(r):
    r['adjudication'].update({'author': None, 'author_type': 'model'})


CHANGES = [
    # The reader ignores an unknown field.
    (unknown_field, None),
    # The reader accepts a newer minor version.
    (newer_minor_version, None),
    (missing_field, ('invalid_finding', 'missing_field')),
    (missing_key_of_an_object, ('invalid_finding', 'missing_key')),
    # The reader refuses an unknown major version.
    (unknown_major_version, ('unsupported_finding_version', 'version_major')),
    (seal_digest_mismatch, ('invalid_finding', 'seal_digest')),
    (seal_file_not_covered, ('invalid_finding', 'seal_coverage')),
    (turn_not_in_script, ('invalid_finding', 'turn_not_in_script')),
    (mismatch_with_model_author, ('invalid_finding', 'field_value')),
]


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
        with tempfile.TemporaryDirectory() as temp:
            work = Path(temp)
            # Each state file of nookku and of a harness stays in the work folder.
            homes = {name: str(work / name.lower()) for name in ['NOOKKU_HOME', 'CODEX_HOME', 'CLAUDE_CONFIG_DIR']}
            for folder in homes.values():
                Path(folder).mkdir()
            with mock.patch.dict(os.environ, homes | {'NOOKKU_CODEX': 'no-codex-command'}):
                cls.exported = export(work)
        cls.library = cls.exported['library']
        # Each case: the name, the exact finding text, and the result that a person expects.
        cls.cases = [(name, text, None) for name, text in cls.exported.items()]
        for change, result in CHANGES:
            record = json.loads(cls.library)
            change(record)
            cls.cases.append((change.__name__, json.dumps(record, ensure_ascii=False), result))

    def test_release(self):
        self.assertEqual(importlib.metadata.version('nookku'), VERSION)
        pin = (ROOT / 'tests/nookku-seam/requirements.txt').read_text('utf-8')
        self.assertIn('nookku==' + VERSION + ' ', pin)
        for text in self.exported.values():
            record = json.loads(text)
            self.assertEqual(record['nookku'], VERSION)
            self.assertEqual((record['record'], record['version']), (finding_record.NAME, '1.0'))

    def test_exported_records(self):
        """The records have the values of the invented example."""
        subject = 'sha256:' + sha256(OLD_CATALOG.encode('utf-8'))
        expected = {'value': '2026-12-09', 'match': 'exact', 'tolerance': None, 'field': 'due'}
        for name in ['library', 'gate_record']:
            with self.subTest(record=name):
                record = json.loads(self.exported[name])
                self.assertEqual(record['kind'], 'answer_key_mismatch')
                self.assertEqual(record['script']['messages'], [MOSS, GARDEN])
                self.assertEqual({k: record['turn'][k] for k in ['number', 'input']}, {'number': 2, 'input': GARDEN})
                self.assertEqual(json.loads(record['turn']['reply'])['due'], '2026-12-16')
                self.assertEqual(record['expected'], expected)
                self.assertEqual(record['adjudication']['author_type'], 'human')
                self.assertEqual(record['adjudication']['reason'], REASON)
                self.assertEqual(record['subject'], subject)
        self.assertIsNone(json.loads(self.library)['gate'])
        self.assertEqual(json.loads(self.exported['gate_record'])['gate']['name'], 'due-dates')
        issue = json.loads(self.exported['issue_row'])
        self.assertEqual(issue['kind'], 'issue_row')
        self.assertIsNone(issue['expected'])
        self.assertEqual(issue['issue'], {'class': 'unsupported_reply', 'evidence': 'turn 2', 'text': ISSUE})
        self.assertEqual(issue['adjudication']['author_type'], 'human')

    def test_release_reader_agrees(self):
        """The reader of the release refuses a record exactly where this test expects a refusal."""
        for name, text, result in self.cases:
            with self.subTest(case=name):
                (row,) = finding_record.check_lines((text + '\n').encode('utf-8'))
                self.assertEqual(row['problem'] is None, result is None, row['problem'])

    def test_cases_are_different(self):
        """Each case has its own finding text, so no change is lost."""
        self.assertEqual(len({text for _, text, _ in self.cases}), len(self.cases))
        self.assertEqual(len(self.cases), 3 + len(CHANGES))

    def test_host_import(self):
        cases = [{'name': name, 'finding': text, 'expect': expectation(text, json.loads(text), result)}
                 for name, text, result in self.cases]
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
