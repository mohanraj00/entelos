// The browser test of the approval dashboard. run.py serves this file with the dashboard, on the
// origin of the API. The harness drives the dashboard in a frame as a person does: it types, presses
// and reads the text that the page shows. The expected values are hand-written from APPROVAL.md and
// the seed steps of run.py. At the end, the harness sends its observations to run.py.
import { define } from '/mugham/src/components/index.js';
import {
  verdictKind, verdictStatus, verdictCard, listCard, listCards, waitText, put, fill, reviewerActor, producerActor,
} from '/view.js';

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
// The card elements in a container of the page. Each card has its own bound author.
const gridCards = (box) => [...(box?.querySelectorAll(':scope > mu-card') ?? [])];
const press = (node, part = 'button') => node.shadowRoot.querySelector(`[part="${part}"]`).click();
// Hand-written: the author line of the kit, `Author: <type> <id>`.
const author = (type, id) => `Author: ${type} ${id}`;

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
  await until('kit defined', () => frame.contentWindow.customElements.get('mu-card'));

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
  check('list-state', cards.every((card) => card.dataset.state === 'default'), cards.map((card) => card.dataset.state));
  check('list-author-producer', texts.every((t) => t.includes(author('agent', 'author'))), texts.map((t) => t.slice(-40)));
  check('stale-shown-as-stale', texts[1].includes('Stale:') && !texts[0].includes('Stale:'), texts.map((t) => t.slice(0, 80)));

  // The evidence view of the oldest proposal.
  const open = doc.querySelector('#open-buttons mu-button');
  check('first-open-button', shadow(open).includes('Open record.visible'), shadow(open));
  press(open);
  await until('detail view', () => !doc.getElementById('detail-view').hidden && doc.getElementById('evidence').fill);
  const evidence = doc.getElementById('evidence');
  check('evidence-reason-as-text', shadow(evidence).includes(job.markup_reason) && !evidence.shadowRoot.querySelector('b'),
    shadow(evidence));
  check('evidence-author-producer', shadow(evidence).includes(author('agent', 'author')), shadow(evidence));
  check('source-author-attester', shadow(doc.getElementById('source')).includes(author('human', 'curator')), shadow(doc.getElementById('source')));
  check('evidence-facts', ['Wait3 d 0 h', 'VerdictWaits for a human verdict', 'Producerauthor', 'Interval2026-10-01 to 2026-12-01', 'Scopedemo']
    .every((s) => shadow(evidence).includes(s)), shadow(evidence));
  const bundle = [...doc.querySelectorAll('#bundle mu-text')].map(shadow);
  check('evidence-bundle-text', bundle.includes('source') && bundle.some((t) => t.trim() === RULE), bundle.slice(0, 8));
  const reports = [...doc.querySelectorAll('#reports mu-card')].map(shadow);
  check('evidence-report', reports.length === 1 && reports[0].includes('Report 1: passed') && reports[0].includes('Cases passed2 of 2')
    && reports[0].includes('Runnerfixture-runner') && reports[0].includes('Report refusalnone')
    && reports[0].includes(author('system', 'fixture-runner')), reports);
  const verdicts = doc.getElementById('verdicts');
  check('verdicts-empty', gridCards(verdicts).length === 0 && verdicts.textContent === '' && shadow(verdicts.querySelector('mu-text')).trim() === 'No verdict.',
    shadow(verdicts.querySelector('mu-text')));

  // A verdict without a kept reason does not call the API.
  const accept = doc.getElementById('accept');
  const reject = doc.getElementById('reject');
  check('verdict-buttons-ready', accept.dataset.state === 'default' && reject.dataset.state === 'default',
    [accept.dataset.state, shadow(accept), reject.dataset.state]);
  // The confirm step: Cancel sends no action.
  press(accept);
  await until('accept confirm step', () => accept.dataset.state === 'confirm');
  const step = accept.shadowRoot.querySelector('[part="confirm"]');
  check('accept-confirm-step', step?.getAttribute('role') === 'group' && step.textContent.includes('with the kept reason?')
    && step.textContent.includes('Confirm accept'), step?.textContent);
  press(accept, 'cancel');
  await until('confirm step closed', () => accept.dataset.state === 'default');
  check('cancel-no-action', !accept.shadowRoot.querySelector('[part="confirm"]') && gridCards(verdicts).length === 0, accept.dataset.state);
  press(accept);
  await until('accept confirm step again', () => accept.dataset.state === 'confirm');
  press(accept, 'accept');
  await until('accept error without reason', () => accept.dataset.state === 'error');
  check('no-reason-no-call', shadow(accept).includes('Keep a reason first.') && gridCards(verdicts).length === 0, shadow(accept));

  // Keep the reason with the editable form.
  const form = doc.getElementById('reason');
  const field = form.shadowRoot.querySelector('mu-field');
  const input = field.shadowRoot.querySelector('input');
  input.value = job.verdict_reason;
  input.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
  check('form-author-page', shadow(form).includes(author('system', 'approval-dashboard')), shadow(form));
  press(form);
  await until('reason kept', () => form.dataset.state === 'done');

  // One verdict: accept, after the confirm step. The button is pending until the API receipt comes back.
  press(accept);
  await until('accept confirm step for the verdict', () => accept.dataset.state === 'confirm');
  press(accept, 'accept');
  check('accept-pending-before-receipt', accept.dataset.state === 'pending', accept.dataset.state);
  await until('accept done', () => accept.dataset.state === 'done' || accept.dataset.state === 'error');
  const status = shadow(accept);
  const shown = accept.shadowRoot.querySelector('[part="reference"]')?.textContent ?? '';
  const reference = (shown.match(/^Reference: (sha256:[0-9a-f]{64})$/) ?? [])[1] ?? null;
  out.receipt = { reference };
  check('accept-done-with-reference', accept.dataset.state === 'done' && reference, status);
  const recorded = await until('verdict card', () => gridCards(verdicts).length === 1 && shadow(gridCards(verdicts)[0]));
  check('verdict-record-shown', recorded.startsWith('Human verdict: accept') && recorded.includes('Actor' + job.reviewer)
    && recorded.includes('Kindhuman') && recorded.includes(job.verdict_reason) && recorded.includes(author('human', job.reviewer)), recorded);
  check('evidence-state-accepted', shadow(evidence).includes('VerdictAccepted'), shadow(evidence));

  // One API refusal: the same actor twice on the same snapshot.
  press(reject);
  await until('reject confirm step', () => reject.dataset.state === 'confirm');
  press(reject, 'accept');
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
  box.append(card);
  put(card, verdictCard(policy, 'sha256:' + '0'.repeat(64)));
  check('unit-policy-card', card.dataset.state === 'default' && shadow(card).startsWith('Policy verdict: route')
    && shadow(card).includes('does not count as a human verdict') && shadow(card).includes(author('system', 'policy')), shadow(card));
  const humanCard = document.createElement('mu-card');
  box.append(humanCard);
  put(humanCard, verdictCard(human, 'sha256:' + '1'.repeat(64)));
  check('unit-human-card', shadow(humanCard).startsWith('Human verdict: reject') && !shadow(humanCard).includes('policy')
    && shadow(humanCard).includes(author('human', 'reviewer-a')), shadow(humanCard));
  check('unit-empty-list', listCards([]).length === 0, listCards([]));

  // The pinned kit refuses a fill of contract format 1, with an actions key.
  const item = { function: 'record.visible', reason: 'r', producer: 'author', state: 'awaiting_verdict', snapshot_ref: 'sha256:' + '2'.repeat(64) };
  const legacy = document.createElement('mu-card');
  box.append(legacy);
  put(legacy, { ...listCard(item), actions: [] });
  check('unit-actions-key-refused', legacy.dataset.state === 'refused' && shadow(legacy).includes('actions: unknown key'), shadow(legacy));
  // The kit refuses a fill with no authorship slot, and a fill whose author is not the bound actor.
  const anonymous = document.createElement('mu-card');
  box.append(anonymous);
  anonymous.actor = producerActor('author');
  anonymous.fill = fill('card', { title: 'record.visible' });
  check('unit-no-author-refused', anonymous.dataset.state === 'refused' && shadow(anonymous).includes('slots.authorship'), shadow(anonymous));
  const claimed = document.createElement('mu-card');
  box.append(claimed);
  claimed.actor = producerActor('author');
  claimed.fill = fill('card', { title: 'record.visible' }, reviewerActor('author'));
  check('unit-other-author-refused', claimed.dataset.state === 'refused' && shadow(claimed).includes('not the bound actor'), shadow(claimed));
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
