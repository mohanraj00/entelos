#!/usr/bin/env python3
"""Check proposed decision-event vectors. No Host or transport runs."""
import argparse
import copy
import json
import os
from pathlib import Path
import re
import sys
import tempfile

HERE = Path(__file__).resolve().parent
# Prevent the existing validator's module from creating a temporary directory.
os.environ.setdefault('PROTOCOL_TEST_WORK', tempfile.gettempdir())
sys.path.insert(0, str(HERE.parent))
from test_protocol import SchemaError, validate  # noqa: E402

SCHEMA = json.loads((HERE / 'schema/event.schema.json').read_text())
SPEC = (HERE / 'EVENTS.md').read_text()
FORMAT = re.compile(r'entelos\.decision-event/(0|[1-9][0-9]*)')
CURSOR = re.compile(r'ev1:([0-9a-f]{64}):(0|[1-9][0-9]{0,15})')
MAX_POSITION = 9007199254740991


def shape(name, value):
    validate(SCHEMA['$defs'][name], value, name, SCHEMA['$defs'])


def position(cursor, stream=None):
    match = CURSOR.fullmatch(cursor) if isinstance(cursor, str) else None
    if not match or int(match[2]) > MAX_POSITION:
        return None
    if stream is not None and match[1] != stream:
        return None
    return int(match[2])


def judge(event):
    """The five event rules, in their stated order."""
    def refuse(rule):
        return {'valid': False, 'code': 'invalid_event', 'rule': rule}

    match = FORMAT.fullmatch(event.get('format', '')) if isinstance(event, dict) and isinstance(event.get('format'), str) else None
    if not match:
        return refuse('format')
    if match[1] != '1':
        return {'valid': False, 'code': 'unsupported_event_format'}
    try:
        shape('event', event)
    except SchemaError:
        return refuse('schema')
    if position(event['cursor']) in (None, 0):
        return refuse('schema')
    if ('corrected_value' in event) != (event['disposition'] == 'correct'):
        return refuse('correction')
    if event['actor_kind'] != 'human' and (event['disposition'] == 'correct' or event['blind']):
        return refuse('provenance')
    return {'valid': True}


def project(decision, stream, start):
    """Project a complete synthetic decision; never use an expected event."""
    if not isinstance(decision, dict) or set(decision) != {'decision_ref', 'actor_kind', 'disposition', 'facts', 'before_stream_start'}:
        return None
    if decision['disposition'] not in ('approve', 'reject'):
        return None
    events = []
    for offset, fact in enumerate(decision['facts'], start + 1):
        if not isinstance(fact, dict):
            return None
        if not {'fact_digest', 'evidence_receipt_digest', 'blind'} <= set(fact):
            return None
        if set(fact) - {'fact_digest', 'evidence_receipt_digest', 'blind', 'corrected_value'}:
            return None
        if 'corrected_value' in fact and decision['disposition'] == 'reject':
            return None
        event = {
            'format': 'entelos.decision-event/1',
            'cursor': 'ev1:%s:%d' % (stream, offset),
            'decision_ref': decision['decision_ref'],
            'disposition': 'correct' if 'corrected_value' in fact else decision['disposition'],
            'fact_digest': fact['fact_digest'],
            'evidence_receipt_digest': fact['evidence_receipt_digest'],
            'actor_kind': decision['actor_kind'],
            'blind': fact['blind'],
        }
        if 'corrected_value' in fact:
            event['corrected_value'] = fact['corrected_value']
        if judge(event) != {'valid': True}:
            return None
        events.append(event)
    return events


def read(stream, request):
    """Read checks, exclusive resume and exact page size from EVENTS.md."""
    def refuse(code):
        return {'ok': False, 'code': code}

    try:
        shape('request', request)
    except SchemaError:
        return refuse('invalid_request')
    namespace = stream['namespace']
    decisions = [d for d in stream['decisions'] if not d['before_stream_start']]
    total = sum(len(d['facts']) for d in decisions)
    start = position(request['cursor'], namespace) if 'cursor' in request else 0
    if start is None or start > total:
        return refuse('invalid_cursor')
    end = min(start + request['limit'], total)
    events = []
    offset = 0
    for decision in decisions:
        count = len(decision['facts'])
        if offset < end and offset + count > start:
            projected = project(decision, namespace, offset)
            if projected is None:
                return refuse('unexportable_decision')
            events.extend(e for e in projected if start < position(e['cursor']) <= end)
        offset += count
    return {
        'ok': True, 'events': events,
        'next_cursor': 'ev1:%s:%d' % (namespace, end),
        'stream_start': stream['stream_start'],
    }


def table_results(title):
    section = SPEC.split('\n## ' + title + '\n', 1)[1].split('\n## ', 1)[0]
    return set(re.findall(r'^\| [0-9]+ \|.*\| `([a-z_]+)`(?:, rule `([a-z_]+)`)? \|$', section, re.M))


def result_key(result):
    return result.get('code'), result.get('rule', '')


def expand(value):
    """Expand only the documented long-string marker in event vectors."""
    if isinstance(value, dict):
        if set(value) == {'$text'}:
            return value['$text']['char'] * value['$text']['count']
        return {k: expand(v) for k, v in value.items()}
    if isinstance(value, list):
        return [expand(v) for v in value]
    return value


def run(event_doc, export_doc):
    errors = []
    names = set()
    event_seen, read_seen = set(), set()
    actors, dispositions, marks = set(), set(), set()
    successful = {}
    for kind, doc in (('event', event_doc), ('export', export_doc)):
        if type(doc.get('format_version')) is not int or doc['format_version'] != 1:
            errors.append(kind + ': format_version must be 1')
        for case in doc['cases']:
            name, expect = case['name'], case['expect']
            if name in names:
                errors.append(name + ': duplicate name')
            names.add(name)
            if kind == 'event':
                event = expand(case['event'])
                got = judge(event)
                if expect.get('valid') is False:
                    event_seen.add(result_key(expect))
                elif got == {'valid': True}:
                    actors.add(event['actor_kind'])
                    dispositions.add(event['disposition'])
                    marks.add(event['blind'])
            else:
                stream = doc['streams'][case['stream']]
                before_read = copy.deepcopy(stream)
                got = read(stream, case['request'])
                if stream != before_read:
                    errors.append(name + ': read changed the source decisions')
                if expect.get('ok') is False:
                    read_seen.add(result_key(expect))
                else:
                    try:
                        shape('page', {k: v for k, v in expect.items() if k != 'ok'})
                    except SchemaError as error:
                        errors.append(name + ': ' + error.message)
                    for event in expect['events']:
                        if judge(event) != {'valid': True}:
                            errors.append(name + ': invalid expected event')
                    successful[name] = case
                if 'after' in case:
                    prior = successful.get(case['after'])
                    if prior is None:
                        errors.append(name + ': after must name an earlier successful case')
                    else:
                        before = doc['streams'][prior['stream']]
                        prefix = before['decisions']
                        if (before['namespace'] != stream['namespace']
                                or before['stream_start'] != stream['stream_start']
                                or stream['decisions'][:len(prefix)] != prefix):
                            errors.append(name + ': after must use the same stream prefix')
                        if case['request'].get('cursor') != prior['expect']['next_cursor']:
                            errors.append(name + ': resume cursor differs from previous page')
            # JSON comparison distinguishes false from 0, unlike Python equality.
            if json.dumps(got, sort_keys=True) != json.dumps(expect, sort_keys=True):
                errors.append(name + ': expected ' + json.dumps(expect, sort_keys=True) + ', got ' + json.dumps(got, sort_keys=True))
    for title, seen in (('Event rules', event_seen), ('Read', read_seen)):
        required = table_results(title)
        errors += [title + ': refusal without a case: ' + repr(r) for r in sorted(required - seen)]
        errors += [title + ': refusal not in spec: ' + repr(r) for r in sorted(seen - required)]
    for label, required, seen in (
        ('actor', {'human', 'model', 'policy'}, actors),
        ('disposition', {'approve', 'reject', 'correct'}, dispositions),
        ('blind', {True, False}, marks),
    ):
        errors += [label + ': missing ' + str(x) for x in required - seen]
    # These cases pin the distinct pagination and projection boundaries.
    required_cases = {
        'three-facts-first-page', 'resume-middle', 'resume-end', 'retry-first-page',
        'read-appended', 'repeat-decision-fact', 'empty-stream', 'zero-fact-proposal',
        'whole-decision-before-partial-page', 'policy-approval', 'model-rejection',
        'no-partial-page-on-source-error', 'first-page-stream-start',
        'old-decisions-excluded-read-continues', 'resume-after-old-decisions',
        'only-old-decisions-empty-page', 'append-after-only-old-decisions',
        'old-decisions-consume-no-positions', 'source-error-after-old-decisions',
    }
    errors += ['missing export boundary case: ' + n for n in sorted(required_cases - names)]
    for name, stream in export_doc['streams'].items():
        if not re.fullmatch(r'[0-9a-f]{64}', stream['namespace']):
            errors.append(name + ': invalid stream namespace')
        try:
            shape('stream_start', stream['stream_start'])
        except SchemaError as error:
            errors.append(name + ': ' + error.message)
        in_stream = False
        for decision in stream['decisions']:
            earlier = decision['before_stream_start']
            if type(earlier) is not bool:
                errors.append(name + ': before_stream_start must be a boolean')
            if earlier and in_stream:
                errors.append(name + ': earlier decisions must form a prefix')
            in_stream = in_stream or not earlier
        refs = [d['decision_ref'] for d in stream['decisions']]
        if len(set(refs)) != len(refs):
            errors.append(name + ': duplicate decision reference')
    return errors


def self_test(event_doc, export_doc):
    """Mutate copies only; the on-disk expectations are never written."""
    def named(doc, name):
        return next(c for c in doc['cases'] if c['name'] == name)

    def wrong_digest(e, x):
        named(x, 'three-facts-first-page')['expect']['events'][0]['fact_digest'] = 'sha256:' + 'f' * 64

    def content(e, x):
        named(x, 'three-facts-first-page')['expect']['events'][1]['source_text'] = 'extra'

    def policy_as_human(e, x):
        named(x, 'policy-approval')['expect']['events'][0]['actor_kind'] = 'human'

    def blind(e, x):
        named(x, 'three-facts-first-page')['expect']['events'][0]['blind'] = True

    def reorder(e, x):
        named(x, 'three-facts-first-page')['expect']['events'].reverse()

    def duplicate(e, x):
        page = named(x, 'resume-middle')['expect']['events']
        page.append(copy.deepcopy(page[0]))

    def gap(e, x):
        named(x, 'resume-middle')['expect']['events'] = []

    def cursor(e, x):
        named(x, 'resume-middle')['expect']['next_cursor'] = 'ev1:' + 'a' * 64 + ':2'

    def version(e, x):
        named(e, 'unknown-major-before-schema')['expect'] = {'valid': True}

    def correction(e, x):
        named(x, 'three-facts-first-page')['expect']['events'][1]['corrected_value'] = 'red'

    def source_receipt(e, x):
        x['streams']['three']['decisions'][0]['facts'][0]['evidence_receipt_digest'] = 'sha256:' + 'f' * 64

    def schema_type(e, x):
        named(e, 'blind-not-bool')['expect'] = {'valid': True}

    def missing_stream_start(e, x):
        del named(x, 'first-page-stream-start')['expect']['stream_start']

    def wrong_stream_start(e, x):
        named(x, 'first-page-stream-start')['expect']['stream_start'] = 'other-revision'

    def include_old_decision(e, x):
        x['streams']['with-old']['decisions'][0]['before_stream_start'] = False

    def exclude_new_decision(e, x):
        x['streams']['missing-actor']['decisions'][0]['before_stream_start'] = True

    def changed_stream_start(e, x):
        # An append must keep the start, even if its pages agree on a new value.
        x['streams']['with-old']['stream_start'] = 'other-revision'
        for case in x['cases']:
            if case['stream'] == 'with-old' and case['expect']['ok']:
                case['expect']['stream_start'] = 'other-revision'

    errors = []
    for mutate in (wrong_digest, content, policy_as_human, blind, reorder, duplicate, gap, cursor, version, correction, source_receipt, schema_type,
                   missing_stream_start, wrong_stream_start, include_old_decision, exclude_new_decision, changed_stream_start):
        e, x = copy.deepcopy(event_doc), copy.deepcopy(export_doc)
        mutate(e, x)
        if not run(e, x):
            errors.append('self-test missed ' + mutate.__name__)
    return errors


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    event_doc = json.loads((HERE / 'vectors/event-cases.json').read_text())
    export_doc = json.loads((HERE / 'vectors/export-cases.json').read_text())
    errors = run(event_doc, export_doc) + self_test(event_doc, export_doc)
    print(json.dumps({
        'passed': not errors, 'events': len(event_doc['cases']),
        'exports': len(export_doc['cases']), 'errors': errors,
        'scope': 'Proposed event rules, stream start, projection, order and exclusive cursors on synthetic decisions. No Host or transport ran.',
    }, indent=2))
    return bool(errors)


if __name__ == '__main__':
    raise SystemExit(main())
