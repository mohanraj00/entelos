// The browser test of the approval dashboard. run.py serves this file with the dashboard, on the
// origin of the API. The harness drives the dashboard in a frame as a person does: it types, presses
// and reads the text that the page shows. The expected values are hand-written from APPROVAL.md and
// the seed steps of run.py. At the end, the harness sends its observations to run.py.
import { define } from '/mugham/src/components/index.js';
import { verdictKind, verdictStatus, verdictCard, listGrid, waitText } from '/view.js';

const out = { checks: [], errors: [] };
const check = (name, passed, actual) => out.checks.push({ name, passed: Boolean(passed), actual });
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function until(name, fn, timeout = 20000) {
  const end = performance.now() + timeout;
  while (performance.now() < end) {
    const value = fn();
    if (value) return value;
    await sleep(50);
  }
  throw new Error(`timeout: ${name}`);
}

const shadow = (node) => (node?.shadowRoot ? node.shadowRoot.textContent : '');
// The text of a card grid, with the text of each card in it.
const gridCards = (grid) => [...(grid?.shadowRoot?.querySelectorAll('mu-card') ?? [])];
const press = (node) => node.shadowRoot.querySelector('[part="button"]').click();

// Hand-written: the text of each list card. The wait comes from the seed clocks.
const LIST = [
  { title: 'record.visible', wait: '3 d 0 h', verdict: 'Waits for a human verdict' },
  { title: 'record.export', wait: '2 d 0 h', verdict: 'Stale: the report needs a new qualification' },
  { title: 'record.archive', wait: '1 d 0 h', verdict: 'Waits for a qualification report' },
];
const RULE = 'state == allowed_state && !archived';

async function signIn(doc, token) {
  doc.getElementById('token').value = token;
  doc.getElementById('sign-in').requestSubmit();
}

async function dashboard(job) {
  const frame = document.getElementById('frame');
  const loaded = new Promise((resolve) => frame.addEventListener('load', resolve, { once: true }));
  frame.src = '/';
  await loaded;
  const doc = frame.contentDocument;
  await until('kit defined', () => frame.contentWindow.customElements.get('mu-card-grid'));

  // An unknown token: the API refuses with unauthenticated, and the page shows the refusal.
  await signIn(doc, job.unknown_token);
  const refused = await until('unauthenticated text', () => shadow(doc.querySelector('#sign-in-error mu-text')));
  check('refusal-unauthenticated', refused.includes('unauthenticated') && doc.getElementById('main').hidden, refused);
  check('token-not-kept-in-field', doc.getElementById('token').value === '', doc.getElementById('token').value.length);

  // The list: three cards, the oldest first, each with its wait time and verdict status.
  await signIn(doc, job.token);
  const list = doc.getElementById('list');
  const cards = await until('three cards', () => gridCards(list).length === 3 && gridCards(list));
  const texts = cards.map(shadow);
  check('list-order-and-facts', LIST.every((row, i) => texts[i].startsWith(row.title) && texts[i].includes('Wait' + row.wait)
    && texts[i].includes('Verdict' + row.verdict)), texts);
  check('list-state', list.dataset.state === 'default', list.dataset.state);
  check('stale-shown-as-stale', texts[1].includes('Stale:') && !texts[0].includes('Stale:'), texts.map((t) => t.slice(0, 80)));

  // The evidence view of the oldest proposal.
  const open = doc.querySelector('#open-buttons mu-button');
  check('first-open-button', shadow(open).includes('Open record.visible'), shadow(open));
  press(open);
  await until('detail view', () => !doc.getElementById('detail-view').hidden && doc.getElementById('evidence').fill);
  const evidence = doc.getElementById('evidence');
  check('evidence-reason-as-text', shadow(evidence).includes(job.markup_reason) && !evidence.shadowRoot.querySelector('b'),
    shadow(evidence));
  check('evidence-facts', ['Wait3 d 0 h', 'VerdictWaits for a human verdict', 'Producerauthor', 'Interval2026-10-01 to 2026-12-01', 'Scopedemo']
    .every((s) => shadow(evidence).includes(s)), shadow(evidence));
  const bundle = [...doc.querySelectorAll('#bundle mu-text')].map(shadow);
  check('evidence-bundle-text', bundle.includes('source') && bundle.some((t) => t.trim() === RULE), bundle.slice(0, 8));
  const reports = [...doc.querySelectorAll('#reports mu-card')].map(shadow);
  check('evidence-report', reports.length === 1 && reports[0].includes('Report 1: passed') && reports[0].includes('Cases passed2 of 2')
    && reports[0].includes('Runnerfixture-runner') && reports[0].includes('Report refusalnone'), reports);
  const verdicts = doc.getElementById('verdicts');
  check('verdicts-empty', verdicts.dataset.state === 'empty', verdicts.dataset.state);

  // A verdict without a kept reason does not call the API.
  const accept = doc.getElementById('accept');
  const reject = doc.getElementById('reject');
  check('verdict-buttons-ready', accept.dataset.state === 'default' && reject.dataset.state === 'default',
    [accept.dataset.state, shadow(accept), reject.dataset.state]);
  press(accept);
  await until('accept error without reason', () => accept.dataset.state === 'error');
  check('no-reason-no-call', shadow(accept).includes('Keep a reason first.') && verdicts.dataset.state === 'empty', shadow(accept));

  // Keep the reason with the editable form.
  const form = doc.getElementById('reason');
  const field = form.shadowRoot.querySelector('mu-field');
  const input = field.shadowRoot.querySelector('input');
  input.value = job.verdict_reason;
  input.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
  press(form);
  await until('reason kept', () => form.dataset.state === 'done');

  // One verdict: accept. The button is pending until the API receipt comes back.
  press(accept);
  check('accept-pending-before-receipt', accept.dataset.state === 'pending', accept.dataset.state);
  await until('accept done', () => accept.dataset.state === 'done' || accept.dataset.state === 'error');
  const status = shadow(accept);
  const reference = (status.match(/recorded the verdict (sha256:[0-9a-f]{64})/) ?? [])[1] ?? null;
  out.receipt = { reference };
  check('accept-done-with-reference', accept.dataset.state === 'done' && reference, status);
  const recorded = await until('verdict card', () => gridCards(verdicts).length === 1 && shadow(gridCards(verdicts)[0]));
  check('verdict-record-shown', recorded.startsWith('Human verdict: accept') && recorded.includes('Actor' + job.reviewer)
    && recorded.includes('Kindhuman') && recorded.includes(job.verdict_reason), recorded);
  check('evidence-state-accepted', shadow(evidence).includes('VerdictAccepted'), shadow(evidence));

  // One API refusal: the same actor twice on the same snapshot.
  press(reject);
  await until('reject answered', () => reject.dataset.state === 'error' || reject.dataset.state === 'done');
  check('refusal-duplicate-verdict', reject.dataset.state === 'error' && shadow(reject).includes('duplicate_verdict'), shadow(reject));
  check('accept-stays-done', accept.dataset.state === 'done', accept.dataset.state);
  check('one-verdict-only', gridCards(verdicts).length === 1, gridCards(verdicts).length);

  // Back to the list: the accepted proposal is not open now.
  press(doc.getElementById('back'));
  const after = await until('two cards', () => gridCards(list).length === 2 && gridCards(list).map(shadow));
  check('list-after-verdict', after[0].startsWith('record.export') && after[1].startsWith('record.archive'), after.map((t) => t.slice(0, 40)));
}

// Hand-written cases for the parts that the seed cannot make: a policy verdict and an empty list.
function units() {
  define();
  const policy = { format: 'entelos.policy/1', kind: 'policy', outcome: 'route', reason: 'human_required', date: '2026-11-22' };
  const human = { format: 'entelos.record/1', actor: 'reviewer-a', disposition: 'reject', reason: 'r', decided_at: '2026-11-22T09:00:00Z' };
  check('unit-verdict-kind', verdictKind(policy) === 'policy' && verdictKind(human) === 'human', [verdictKind(policy), verdictKind(human)]);
  check('unit-superseded-is-stale', verdictStatus({ state: 'superseded', refusal: '' }).kind === 'stale', verdictStatus({ state: 'superseded' }));
  check('unit-wait-text', waitText(90) === '1 min' && waitText(3600 * 5 + 60) === '5 h 1 min' && waitText(-5) === 'clock went back by 5 s',
    [waitText(90), waitText(18060), waitText(-5)]);
  const box = document.getElementById('units');
  const card = document.createElement('mu-card');
  card.fill = verdictCard(policy, 'sha256:' + '0'.repeat(64));
  box.append(card);
  check('unit-policy-card', card.dataset.state === 'default' && shadow(card).startsWith('Policy verdict: route')
    && shadow(card).includes('does not count as a human verdict'), shadow(card));
  const humanCard = document.createElement('mu-card');
  humanCard.fill = verdictCard(human, 'sha256:' + '1'.repeat(64));
  box.append(humanCard);
  check('unit-human-card', shadow(humanCard).startsWith('Human verdict: reject') && !shadow(humanCard).includes('policy'), shadow(humanCard));
  const empty = document.createElement('mu-card-grid');
  empty.fill = listGrid([]);
  box.append(empty);
  check('unit-empty-list', empty.dataset.state === 'empty' && shadow(empty).includes('No cards.'), shadow(empty));
}

let job = null;
try {
  job = await (await fetch('job.json', { cache: 'no-store' })).json();
  units();
  await dashboard(job);
} catch (error) {
  out.errors.push(String(error?.stack ?? error));
  check('harness-completed', false, String(error));
}
if (job) location.href = job.result_url + '?data=' + encodeURIComponent(JSON.stringify(out));
