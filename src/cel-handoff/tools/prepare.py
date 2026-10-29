#!/usr/bin/env python3
"""Materialize the approved neutral fixture; bind it to an actual host build."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def digest(media, body):
    return 'sha256:' + hashlib.sha256(media.encode() + b'\0' + body).hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n')


def prepare(directory, binary, fixture):
    directory.mkdir(parents=True, exist_ok=True)
    f = json.loads(fixture.read_text())
    write_json(directory / 'env.json', f['environment'])
    (directory / 'rule.cel').write_text(f['expression'] + '\n')
    write_json(directory / 'fixed.json', f['fixed_bindings'])
    write_json(directory / 'cases.json', f['expected_calls'])
    write_json(directory / 'upstream-tests.json', [
        {'testCase': c['case'], 'bindings': c['inputs'] | f['fixed_bindings'], 'expected': c['value']}
        for c in f['expected_calls'] if c['status'] == 'ok'
    ])
    build = json.loads(subprocess.check_output([str(binary), '-build']))
    limits = f['proposed_profile']['resource_budgets']
    manifest = {
        'function_id': f['function_id'], 'profile': 'cel-string-bool/v1',
        'caller_variables': f['caller_variables'], 'implementation_ref': build['implementation_ref'],
        'limits': {k: int(v) for k, v in limits.items() if k not in ('status', 'cost_model')},
    }
    for key, filename, media in [('environment_ref', 'env.json', 'application/json'),
                                 ('source_ref', 'rule.cel', 'text/plain'),
                                 ('fixed_ref', 'fixed.json', 'application/json')]:
        manifest[key] = digest(media, (directory / filename).read_bytes())
    write_json(directory / 'manifest.json', manifest)
    return manifest


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bundle', type=Path, required=True)
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--fixture', type=Path, default=Path(__file__).resolve().parents[1] / 'testdata/scope-fixture.json')
    args = p.parse_args()
    prepare(args.bundle, args.binary.resolve(), args.fixture)
