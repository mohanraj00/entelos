"""The seam test of #104: a groundgate export, mapped to entelos.evidence/1.

verify.py runs this file in a virtual environment that has only the groundgate release of
tests/groundgate-seam/requirements.txt. The test admits an invented document with groundgate,
records one human answer, resolves the review and exports facts@2. It maps the export and the
store records to an evidence document, as the map table of EVIDENCE.md says. Then the checker
of EVIDENCE.md and the Host evidence read must accept the document and give the policy inputs.

The values, outcomes, codes, spans, statuses and policy inputs below are written by hand from
the invented document. The test takes the digests from the groundgate store and from admit, not
from the map, and compares them with the map.
"""
import contextlib
import copy
import importlib.metadata
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from groundgate.cli import main
from groundgate.review.tasks import write_tasks
from groundgate.store import FolderStore, reviewer_key
from groundgate.store import key as store_key
from groundgate.store.packets import admit_packet, put_reading

ROOT = Path(__file__).resolve().parents[1]
VERSION = '0.16.0'
# The producer rule of the digests. The receipt names it in its "groundgate" member.
DIGEST_SCHEME = 'groundgate/0.12'
DOCUMENT = 'Order note\nBoxes: 3\nEstimate: up to 4\n'
SCHEMA = {'fields': {'boxes': {'type': 'integer'}, 'estimate': {'type': 'integer'}}}
CANDIDATES = [
    {'id': 'a', 'field': 'boxes', 'value': 3, 'evidence': {'start': 18, 'end': 19, 'text': '3'}},
    {'id': 'b', 'field': 'estimate', 'value': 4, 'evidence': {'start': 36, 'end': 37, 'text': '4'}},
]
# groundgate records boxes in hundredths of a PDF point.
PLACES = [[18, 19, 1, 7200, 70050, 9000, 71225]]
REVIEW_POLICY = {'format': 'review-policy@1', 'audit': {'rate': '0', 'seed': ''}, 'agree': 1,
                 'leads': [], 'rejected': False}
REVIEWER = 'ana'
# answer@2 has no reviewer kind. The map states it for each reviewer name.
REVIEWER_KINDS = {REVIEWER: 'human'}
ANSWER = {'format': 'answer@2', 'reviewer': REVIEWER, 'verdict': 'confirm', 'decision': None,
          'candidate': None, 'codes_kept': [], 'statements': None, 'nodes': None,
          'field_keys': None, 'reason_seen': False, 'keys': 1, 'ms': 1000,
          'time': '2026-11-15T10:00:00Z'}
# The facts that a person expects from DOCUMENT, without the digests.
EXPECTED_FACTS = [
    {'fact_id': 'boxes', 'outcome': 'admitted', 'codes': [],
     'spans': [{'start': 18, 'end': 19, 'page': 1,
                'box': {'x0': 72000, 'y0': 700500, 'x1': 90000, 'y1': 712250}}],
     'data': {'field': 'boxes', 'key': None, 'value': '3', 'unit': None}, 'review': None},
    {'fact_id': 'estimate', 'outcome': 'needs_verification', 'codes': ['QUALIFIED_VALUE'],
     'spans': [{'start': 36, 'end': 37}],
     'data': {'field': 'estimate', 'key': None, 'value': '4', 'unit': None},
     'review': {'status': 'confirmed', 'answers': [
         {'reviewer': REVIEWER, 'reviewer_kind': 'human', 'verdict': 'confirm',
          'time': '2026-11-15T10:00:00Z', 'ms': 1000}]}},
]
# One fact came from a human review. An automatic approval policy that refuses a human-reviewed
# fact sends this proposal to a human decide.
EXPECTED_INPUTS = {'evidence_human_reviewed': True, 'evidence_model_reviewed': False,
                   'evidence_all_automatic': False, 'evidence_corrected': False}
# Review statuses that the map can give. A correction needs the correction receipt, and
# this map does not read it.
MAPPED_STATUSES = {'admitted', 'confirmed', 'chosen'}


def load_checker():
    spec = importlib.util.spec_from_file_location('evidence_check_spec',
                                                  ROOT / 'src/protocol/evidence/check_spec.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def cli(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = main([str(a) for a in args])
    return code, out.getvalue()


def to_evidence(folder, export):
    """Map a facts@2 export and the store records of its packet to entelos.evidence/1."""
    if export['open']:
        raise ValueError('the export has open slots')
    with FolderStore(folder) as store:
        resolution = json.loads(store.get('resolution@1', export['resolution_sha256']))
        slots = {(s['field'], s['key']): s for s in resolution['slots']}
        logs = {}
        facts = []
        for f in export['facts']:
            if f['status'] not in MAPPED_STATUSES:
                raise ValueError('the map does not give the status ' + f['status'])
            slot = slots[f['field'], f['key']]
            receipt = json.loads(store.get('receipt@1', slot['receipt']))
            if receipt['receipt_sha256'] != f['citation']['receipt_sha256']:
                raise ValueError('the slot receipt is not the cited receipt')
            (decision,) = [receipt['decisions'][i] for i in slot['decisions']][:1]
            citation = f['citation']
            spans = []
            if citation['span'] is not None:
                span = {k: citation['span'][k] for k in ('start', 'end')}
                locator = citation.get('locator')
                if locator is not None:
                    span['page'] = locator['page']
                    span['box'] = {k: n * 10 for k, n in zip(('x0', 'y0', 'x1', 'y1'), locator['box'])}
                spans.append(span)
            fact = {
                'fact_id': f['field'] if f['key'] is None else f['field'] + ':' + f['key'],
                'receipt_digest': receipt['receipt_sha256'],
                'decision_digest': decision['candidate_sha256'],
                'packet_digest': export['packet_sha256'],
                'source_kind': citation['source'],
                'source_digest': citation['source_sha256'],
                'text_digest': citation['text_sha256'],
                'url': citation['url'],
                'ref': citation['ref'],
                'spans': spans,
                'outcome': decision['outcome'],
                'codes': decision['codes'],
                'data': {'field': f['field'], 'key': f['key'], 'value': f['value'], 'unit': f['unit'],
                         'schema_digest': receipt['schema_sha256']},
            }
            if receipt['policy_sha256'] is not None:
                fact['data']['policy_digest'] = receipt['policy_sha256']
            if f['review'] is not None:
                answers = []
                for a in f['review']['answers']:
                    name = a['reviewer']
                    if name not in logs:
                        logs[name] = store.read(('answers', resolution['packet'], reviewer_key(name)))
                    (record,) = [r for r in logs[name] if r['entry_sha256'] == a['entry_sha256']]
                    answers.append({'reviewer': name, 'reviewer_kind': REVIEWER_KINDS[name],
                                    'verdict': a['verdict'], 'time': record['time'], 'ms': record['ms'],
                                    'entry_digest': a['entry_sha256']})
                fact['review'] = {'status': f['status'], 'answers': answers,
                                  'task_digest': f['review']['task'],
                                  'resolution_digest': export['resolution_sha256']}
            facts.append(fact)
    producer = {'id': 'groundgate', 'version': VERSION, 'digest_scheme': DIGEST_SCHEME}
    if any('box' in span for fact in facts for span in fact['spans']):
        value = {'number_units': 'ppm-milli'}
        raw = json.dumps(value, separators=(',', ':')).encode('utf-8')
        producer['producer_data'] = {'digest': store_key('number-units@1', raw), 'value': value}
    return {'format': 'entelos.evidence/1', 'producer': producer, 'facts': facts}


class GroundgateSeam(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory()
        work = Path(cls.temp.name)
        cls.source = work / 'source.txt'
        cls.source.write_bytes(DOCUMENT.encode('utf-8'))
        cls.folder = work / 'store'
        policy = work / 'review-policy.json'
        policy.write_text(json.dumps(REVIEW_POLICY), 'utf-8')
        with FolderStore(cls.folder) as store:
            reading, _ = put_reading(store, cls.source)
            cls.packet, receipt = admit_packet(store, DOCUMENT, SCHEMA, CANDIDATES, None, reading=reading, places=PLACES)
            cls.receipt = receipt.to_dict()
            cls.tasks = write_tasks(store, cls.packet, REVIEW_POLICY).tasks
            for task, _ in cls.tasks:
                store.append(('answers', cls.packet, reviewer_key(REVIEWER)), dict(ANSWER, task=task))
        code, out = cli('review', 'resolve', '--store', cls.folder, cls.packet, '--policy', policy,
                        '--reviewer', REVIEWER)
        assert code == 0, out
        cls.resolution = out.split()[0]
        facts = work / 'facts.json'
        code, out = cli('export', '--store', cls.folder, cls.packet, '--resolution', cls.resolution,
                        '-o', facts)
        assert code == 0, out
        cls.export = json.loads(facts.read_text('utf-8'))
        cls.evidence = to_evidence(cls.folder, cls.export)
        cls.checker = load_checker()

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def test_release(self):
        self.assertEqual(importlib.metadata.version('groundgate'), VERSION)
        self.assertEqual(self.receipt['groundgate'], DIGEST_SCHEME.split('/')[1])
        self.assertEqual(cli('verify', '--store', self.folder, self.packet)[0], 0)
        self.assertEqual(self.export['format'], 'facts@2')
        self.assertEqual([t['slot']['field'] for _, t in self.tasks], ['estimate'])

    def test_map_values(self):
        got = []
        for f in self.evidence['facts']:
            review = f.get('review')
            if review is not None:
                review = {'status': review['status'],
                          'answers': [{k: a[k] for k in ['reviewer', 'reviewer_kind', 'verdict', 'time', 'ms']}
                                      for a in review['answers']]}
            got.append({'fact_id': f['fact_id'], 'outcome': f['outcome'], 'codes': f['codes'],
                        'spans': f['spans'], 'review': review,
                        'data': {k: f['data'][k] for k in ['field', 'key', 'value', 'unit']}})
        self.assertEqual(sorted(got, key=lambda f: f['fact_id']), EXPECTED_FACTS)

    def test_map_digests(self):
        decisions = {d['field']: d['candidate_sha256'] for d in self.receipt['decisions']}
        with FolderStore(self.folder) as store:
            (entry,) = [r['entry_sha256'] for r in
                        store.read(('answers', self.packet, reviewer_key(REVIEWER)))]
        for f in self.evidence['facts']:
            with self.subTest(fact=f['fact_id']):
                self.assertEqual(f['receipt_digest'], self.receipt['receipt_sha256'])
                self.assertEqual(f['decision_digest'], decisions[f['data']['field']])
                self.assertEqual(f['packet_digest'], self.packet)
                self.assertEqual(f['text_digest'], self.receipt['document']['sha256'])
                self.assertEqual(f['source_digest'], store_key('source@1', self.source.read_bytes()))
                self.assertEqual(f['data']['schema_digest'], self.receipt['schema_sha256'])
                if 'review' in f:
                    self.assertEqual(f['review']['resolution_digest'], self.resolution)
                    self.assertEqual(f['review']['task_digest'], self.tasks[0][0])
                    self.assertEqual([a['entry_digest'] for a in f['review']['answers']], [entry])

    def cases(self):
        """The mapped document, and two changed copies that a rule refuses."""
        unreviewed = copy.deepcopy(self.evidence)
        for f in unreviewed['facts']:
            f.pop('review', None)
        corrected = copy.deepcopy(self.evidence)
        for f in corrected['facts']:
            if 'review' in f:
                f['review']['status'] = 'corrected'
        return [
            ('mapped', self.evidence, {'valid': True, 'inputs': EXPECTED_INPUTS}),
            ('review-removed', unreviewed, {'valid': False, 'code': 'invalid_evidence', 'rule': 'unreviewed_fact'}),
            ('corrected-without-receipt', corrected,
             {'valid': False, 'code': 'invalid_evidence', 'rule': 'correction_receipt_required'}),
        ]

    def test_checker(self):
        for name, doc, expect in self.cases():
            with self.subTest(case=name):
                self.assertEqual(self.checker.judge(doc), expect)

    def test_host_evidence_read(self):
        cases = []
        for name, doc, expect in self.cases():
            want = {'admission': 'valid' if expect['valid'] else 'invalid'}
            want.update({k: expect[k] for k in ['code', 'rule', 'inputs'] if k in expect})
            cases.append({'name': name, 'evidence': doc, 'expect': want})
        path = Path(self.temp.name) / 'seam-cases.json'
        path.write_text(json.dumps({'cases': cases}), 'utf-8')
        result = subprocess.run(['go', 'test', '-mod=readonly', '-count=1', '-v', '-run', '^TestEvidenceSeam$', '.'],
                                cwd=ROOT / 'src/lifecycle-core', capture_output=True, text=True, timeout=300,
                                env=os.environ | {'ENTELOS_EVIDENCE_SEAM': str(path)})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('--- PASS: TestEvidenceSeam', result.stdout)


if __name__ == '__main__':
    unittest.main()
