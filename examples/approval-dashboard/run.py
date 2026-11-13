#!/usr/bin/env python3
"""Run the approval dashboard against the HTTP approval API, and test it in a headless browser.

The script fetches the mugham kit at the pin of mugham.json, builds the site directory, starts a
sidecar with the HTTP binding, local static tokens and --http-static, and puts three synthetic
proposals in the store through the API. The browser test then loads test/harness.html. The harness
drives the dashboard in a frame: the list, the evidence view, one accept and one API refusal. The
expected values are hand-written from APPROVAL.md and the seed steps below. No page output made them.
With --serve, the script keeps the sidecar open for a person and writes the tokens to a private file.
"""
import argparse
import hashlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import subprocess
import sys
import threading
import urllib.error
import urllib.parse
import urllib.request

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SCHEMA = ROOT / 'src/protocol/schema'
PIN = json.loads((HERE / 'mugham.json').read_text())
INTERVAL = {'from': '2026-10-01', 'until': '2026-12-01'}
# Each principal has one actor. The curator writes the sources and the plans.
PRINCIPALS = {'curator': ['reviewer'], 'author': ['author'], 'fixture-runner': ['runner'],
              'reviewer-a': ['reviewer'], 'reviewer-b': ['reviewer']}
FUNCTIONS = ['record.visible', 'record.export', 'record.archive']
READ_CLOCK = '2026-11-23T09:00:00Z'
MARKUP_REASON = 'Initial proposal. <b>This stays text</b>.'


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


sidecar_client = module('sidecar_client', ROOT / 'examples/sidecar-client/run.py')
browser_check = module('browser_check', ROOT / 'src/cel-handoff/web/browser_check.py')


def git(args, cwd=None):
    result = subprocess.run(['git'] + args, cwd=cwd, capture_output=True, text=True, timeout=300)
    if result.returncode:
        raise RuntimeError('git %s: %s' % (' '.join(args), result.stderr.strip()))
    return result.stdout.strip()


def fetch_kit(work, given):
    """A clean checkout of the pinned tag. The tag and HEAD must both name the pinned commit."""
    kit = given.resolve() if given else work / 'mugham'
    if not given:
        git(['clone', '--quiet', '--no-checkout', PIN['repository'], str(kit)])
        git(['checkout', '--quiet', '--detach', 'refs/tags/' + PIN['tag']], kit)
    tag = git(['rev-parse', 'refs/tags/%s^{commit}' % PIN['tag']], kit)
    head = git(['rev-parse', 'HEAD'], kit)
    status = git(['status', '--porcelain'], kit)
    if tag != PIN['commit'] or head != PIN['commit'] or status:
        raise RuntimeError('the mugham checkout is not the clean pin: tag %s, HEAD %s, status %r' % (tag, head, status))
    return kit, {'tag': PIN['tag'], 'commit': head}


def build_site(work, kit, job):
    """The served directory: the dashboard, the kit under mugham/ and, for a test, the harness."""
    site = work / 'site'
    shutil.copytree(HERE / 'site', site)
    for name in PIN['paths']:
        source = kit / name
        (shutil.copytree if source.is_dir() else shutil.copyfile)(source, site / 'mugham' / name)
    if job is not None:
        shutil.copytree(HERE / 'test', site / 'test')
        (site / 'test/job.json').write_text(json.dumps(job, indent=1) + '\n')
    return site


def private(path, text):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
        f.write(text)


def credentials(work):
    """Random tokens and the credentials file of APPROVAL.md. The file has only the digests."""
    tokens = {actor: secrets.token_hex(32) for actor in PRINCIPALS}
    principals = [{'token_sha256': hashlib.sha256(tokens[actor].encode()).hexdigest(), 'actor': actor, 'roles': roles}
                  for actor, roles in PRINCIPALS.items()]
    private(work / 'credentials.json', json.dumps({'format': 'entelos.credentials/1', 'principals': principals}))
    return tokens


class Sidecar:
    def __init__(self, binary, runtime, work, site):
        context = {'scope': 'demo', 'runner_actor': 'fixture-runner', 'worker': 'local-worker', 'functions': FUNCTIONS}
        (work / 'context.json').write_bytes(sidecar_client.encode(context))
        self.clock = work / 'clock'
        self.clock.write_text('2026-11-20T09:00:00Z\n')
        self.log = open(work / 'sidecar.log', 'wb')
        self.args = [str(binary), '--socket', 'dashboard.sock', '--repo', str(work / 'state.git'), '--context', str(work / 'context.json'),
                     '--runtime', str(runtime), '--work', str(work / 'sidecar'), '--schema', str(SCHEMA),
                     '--credentials-file', str(work / 'credentials.json'), '--clock-file', str(self.clock),
                     '--http-listen', '127.0.0.1:0', '--http-local-tokens', '--http-static', str(site)]
        self.process = subprocess.Popen(self.args, cwd=work / 'sockets', stdout=subprocess.PIPE, stderr=self.log)
        ready = json.loads(self.process.stdout.readline() or b'null') or {}
        if ready.get('status') != 'listening' or not ready.get('http'):
            self.stop()
            raise RuntimeError('the sidecar did not start: %s' % (work / 'sidecar.log').read_text())
        self.base = 'http://' + ready['http']

    def at(self, value):
        self.clock.write_text(value + '\n')

    def stop(self):
        self.process.terminate()
        try:
            code = self.process.wait(timeout=30)
        except subprocess.TimeoutExpired:
            self.process.kill()
            code = self.process.wait()
        self.log.close()
        return code


class Api:
    """A plain HTTP client of the approval API. It imports no lifecycle or dashboard code."""

    def __init__(self, base, tokens):
        self.base, self.tokens, self.counter, self.cursor, self.steps = base, tokens, 0, '', []

    def call(self, actor, operation, body, write=False):
        self.counter += 1
        if write:
            body = dict(body, expected_revision=self.cursor, request_key='seed-%d' % self.counter)
        request = {'protocol': '1.1', 'request_id': 'seed-%d' % self.counter, 'operation': operation, 'body': body}
        http = urllib.request.Request(self.base + '/v1/request', data=json.dumps(request).encode(), method='POST',
                                      headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + self.tokens[actor]})
        try:
            with urllib.request.urlopen(http, timeout=60) as reply:
                status, response = reply.status, json.loads(reply.read())
        except urllib.error.HTTPError as error:
            status, response = error.code, json.loads(error.read())
        self.steps.append({'actor': actor, 'operation': operation, 'http_status': status, 'status': response.get('status'),
                           'diagnostic': response.get('diagnostic')})
        if response.get('status') != 'ok':
            raise RuntimeError('%s by %s: %s' % (operation, actor, response))
        if write:
            self.cursor = response['result']['cursor']
            return response['result']['result']
        return response['result']


def function_bundle(record_bundle, function, state):
    """The record bundle with its manifest renamed to one function. The rule, the environment and the
    implementation stay the same, so each function has the same build identity."""
    files = sidecar_client.bundle(record_bundle, state)
    manifest = json.loads(files['manifest'])
    manifest['function_id'] = function
    files['manifest'] = sidecar_client.encode(manifest).decode()
    return files


def seed(api, sidecar, record_bundle):
    """Three open proposals: one ready for a verdict, one with a stale report and one with no report."""
    refs = {}
    for index, function in enumerate(FUNCTIONS):
        state = ['ready', 'open', 'closed'][index]
        source = api.call('curator', 'source', dict(INTERVAL, function=function, supersedes=[], reason='initial requirement',
                          text='Invented source table: visible state %s; archived records are invisible.' % state), True)
        cases = [{'case': 'visible', 'inputs': {'state': state, 'archived': False}, 'status': 'ok', 'value': True},
                 {'case': 'archived', 'inputs': {'state': state, 'archived': True}, 'status': 'ok', 'value': False}]
        plan = api.call('curator', 'plan', {'source_ref': source['source_ref'], 'cases': cases, 'supersedes': [],
                                            'origin': 'Hand-written cases from the invented source table'}, True)
        refs[function] = {'source_ref': source['source_ref'], 'plan_ref': plan['plan_ref'], 'cases': cases, 'state': state}
    for index, function in enumerate(FUNCTIONS):
        sidecar.at('2026-11-%02dT09:00:00Z' % (20 + index))
        reason = MARKUP_REASON if index == 0 else 'Initial proposal.'
        proposal = api.call('author', 'propose', dict(INTERVAL, function=function, base_snapshot='', source_ref=refs[function]['source_ref'],
                            bundle=function_bundle(record_bundle, function, refs[function]['state']), reason=reason), True)
        refs[function]['snapshot_ref'] = proposal['snapshot_ref']
        if index < 2:
            report = api.call('fixture-runner', 'qualify', {'snapshot_ref': proposal['snapshot_ref'], 'plan_ref': refs[function]['plan_ref']}, True)
            refs[function]['report_ref'] = report['report_ref']
    # A new plan supersedes the plan of the second report, so that report is stale.
    function = FUNCTIONS[1]
    api.call('curator', 'plan', {'source_ref': refs[function]['source_ref'], 'cases': refs[function]['cases'],
                                 'supersedes': [refs[function]['plan_ref']], 'origin': 'Hand-written cases, second edition'}, True)
    sidecar.at(READ_CLOCK)
    return refs


# The expected list, hand-written from APPROVAL.md and the seed steps: the oldest first, the wait from
# the read clock, and the state of each proposal.
EXPECTED_LIST = [
    {'function': 'record.visible', 'wait_seconds': 259200, 'state': 'awaiting_verdict', 'refusal': ''},
    {'function': 'record.export', 'wait_seconds': 172800, 'state': 'not_approvable', 'refusal': 'stale_qualification'},
    {'function': 'record.archive', 'wait_seconds': 86400, 'state': 'awaiting_qualification', 'refusal': ''},
]


def listed(api, actor):
    return [{k: item.get(k) for k in ['function', 'wait_seconds', 'state', 'refusal']}
            for item in api.call(actor, 'proposals', {})['proposals']]


def collect(timeout):
    """A local server on another port that gets the result of the harness as a navigation."""
    received = {'event': threading.Event()}

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            url = urllib.parse.urlsplit(self.path)
            data = urllib.parse.parse_qs(url.query).get('data')
            if url.path != '/result' or not data or 'result' in received:
                self.send_error(400)
                return
            received['result'] = json.loads(data[0])
            body = b'<!doctype html><title>result</title>received'
            self.send_response(200)
            self.send_header('Content-Type', 'text/html; charset=utf-8')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            received['event'].set()

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, received


def run_browser(browser, url, work, received, timeout):
    version = subprocess.run([browser, '--version'], capture_output=True, text=True, timeout=60).stdout.strip()
    args = [browser, '--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-extensions', '--disable-gpu',
            '--user-data-dir=%s' % (work / 'profile'), url]
    if os.environ.get('ENTELOS_BROWSER_NO_SANDBOX') == '1':
        args.insert(1, '--no-sandbox')
    with open(work / 'browser.log', 'wb') as log:
        process = subprocess.Popen(args, stdout=log, stderr=log, start_new_session=True)
        try:
            received['event'].wait(timeout)
        finally:
            try:
                os.killpg(process.pid, signal.SIGTERM)
                process.wait(timeout=30)
            except (ProcessLookupError, subprocess.TimeoutExpired):
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
    return {'browser': browser, 'version': version, 'no_sandbox': '--no-sandbox' in args}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--work', type=Path, required=True, help='new directory outside this source tree')
    p.add_argument('--sidecar', type=Path, required=True, help='entelos-sidecar executable')
    p.add_argument('--cel-binary', type=Path, required=True, help='pinned cel-handoff executable')
    p.add_argument('--record-bundle', type=Path, required=True, help='bundle from src/cel-handoff/tools/prepare.py')
    p.add_argument('--mugham', type=Path, help='a clean mugham checkout at the pin; else the script clones it')
    p.add_argument('--browser', help='Chromium executable; else ENTELOS_BROWSER or a known name')
    p.add_argument('--timeout', type=float, default=180)
    p.add_argument('--serve', action='store_true', help='keep the dashboard open for a person; stop with Ctrl-C')
    a = p.parse_args()
    w = a.work.resolve()
    if w == ROOT or ROOT in w.parents:
        p.error('work must be outside this source tree')
    if w.exists():
        p.error('%s already exists; choose a new path' % w)
    (w / 'sockets').mkdir(parents=True)
    os.chmod(w / 'sockets', 0o700)
    checks, error, record, sidecar, server = [], None, {}, None, None

    def check(name, passed, actual):
        checks.append({'name': name, 'passed': bool(passed), 'actual': actual})

    try:
        kit, record['mugham'] = fetch_kit(w, a.mugham)
        tokens = credentials(w)
        browser = None if a.serve else browser_check.find_browser(a.browser)
        if not a.serve and not browser:
            raise RuntimeError('no Chromium browser found; set ENTELOS_BROWSER')
        job = None
        if not a.serve:
            server, received = collect(a.timeout)
            job = {'token': tokens['reviewer-a'], 'unknown_token': secrets.token_hex(32), 'reviewer': 'reviewer-a',
                   'result_url': 'http://127.0.0.1:%d/result' % server.server_address[1],
                   'expected_list': EXPECTED_LIST, 'markup_reason': MARKUP_REASON, 'verdict_reason': 'Reviewed the evidence and the passed report.'}
        site = build_site(w, kit, job)
        sidecar = Sidecar(a.sidecar.resolve(), a.cel_binary.resolve(), w, site)
        api = Api(sidecar.base, tokens)
        refs = seed(api, sidecar, a.record_bundle.resolve())
        record['refs'] = refs
        check('api-list-before', listed(api, 'reviewer-b') == EXPECTED_LIST, listed(api, 'reviewer-b'))
        if a.serve:
            private(w / 'tokens.json', json.dumps({actor: tokens[actor] for actor in ['reviewer-a', 'reviewer-b']}, indent=1) + '\n')
            print(json.dumps({'url': sidecar.base + '/', 'tokens': str(w / 'tokens.json')}), flush=True)
            try:
                signal.sigwait({signal.SIGINT, signal.SIGTERM})
            except KeyboardInterrupt:
                pass
            return 0
        record['browser'] = run_browser(browser, sidecar.base + '/test/harness.html', w, received, a.timeout)
        result = received.get('result')
        record['page'] = result
        check('page-result', result is not None, 'the harness posted a result' if result else 'no result in %ss' % a.timeout)
        for item in (result or {}).get('checks', []):
            check('page-' + item['name'], item['passed'], item.get('actual'))
        # The API, not the page, confirms the verdict record and the refusal.
        visible = refs[FUNCTIONS[0]]['snapshot_ref']
        detail = api.call('reviewer-b', 'proposal', {'snapshot_ref': visible})
        decisions = [entry['decision'] for entry in detail['decisions']]
        expected = [{'actor': 'reviewer-a', 'disposition': 'accept', 'reason': job['verdict_reason'], 'decided_at': READ_CLOCK}]
        check('api-verdict-record', [{k: d.get(k) for k in expected[0]} for d in decisions] == expected
              and detail['proposal']['state'] == 'accepted', {'decisions': decisions, 'state': detail['proposal']['state']})
        page_ref = ((result or {}).get('receipt') or {}).get('reference')
        check('page-receipt-is-the-record', [e['decision_ref'] for e in detail['decisions']] == [page_ref],
              {'page': page_ref, 'api': [e['decision_ref'] for e in detail['decisions']]})
        check('api-list-after', listed(api, 'reviewer-b') == EXPECTED_LIST[1:], listed(api, 'reviewer-b'))
    except Exception as e:
        error = '%s: %s' % (type(e).__name__, e)
    finally:
        if sidecar:
            check('sidecar-shutdown', sidecar.stop() == 0, 'exit code')
        if server:
            server.shutdown()
    passed = error is None and bool(checks) and all(x['passed'] for x in checks)
    result = {'kind': 'ApprovalDashboardExample', 'status': 'passed' if passed else 'failed', 'error': error,
              'checks': checks, 'record': record, 'work': str(w)}
    (w / 'results.json').write_text(json.dumps(result, indent=2, ensure_ascii=False) + '\n')
    print(json.dumps({'status': result['status'], 'error': error, 'checks': len(checks),
                      'failed': [x['name'] for x in checks if not x['passed']]}))
    return 0 if passed else 1


if __name__ == '__main__':
    sys.exit(main())
