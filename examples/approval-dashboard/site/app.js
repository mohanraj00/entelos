// The approval dashboard. It reads and changes state only through the HTTP approval API.
// The mugham kit shows the data. The kit is optional: the API is the contract.
import { define } from './mugham/src/components/index.js';
import { call } from './api.js';
import {
  fill, put, clip, excerpt, listCards, proposalCard, sourceCard, reportCard, verdictCards, refusalText, shortRef, PAGE_ACTOR,
} from './view.js';

define();

const MAX_TEXT = 10000;
const $ = (id) => document.getElementById(id);
const make = (tag, value) => {
  const node = document.createElement(tag);
  if (value) node.fill = value;
  return node;
};
const note = (text, variant = 'body') => make('mu-text', fill('text', { text: clip(text, MAX_TEXT), variant }));
// One card element for each fill, each with its own bound author. With no fill, the target shows the empty text.
const cards = (target, fills, empty) => target.replaceChildren(...(fills.length
  ? fills.map((value) => put(document.createElement('mu-card'), value))
  : [note(empty, 'caption')]));

// The token stays in this variable only. The page does not store it.
let token = '';
// The cursor of the last API result. A write sends it as expected_revision.
let cursor = '';
// The reason that the form kept. The verdict buttons send only this value.
let kept = '';
let current = null;

function show(view) {
  $('list-view').hidden = view !== 'list';
  $('detail-view').hidden = view !== 'detail';
}

function alertText(target, text) {
  target.replaceChildren(text ? note(text, 'strong') : '');
}

$('sign-in').addEventListener('submit', (event) => {
  event.preventDefault();
  token = $('token').value.trim();
  $('token').value = '';
  loadList();
});

async function loadList() {
  alertText($('list-error'), '');
  const response = await call(token, 'proposals', {});
  if (response.status !== 'ok') {
    $('main').hidden = response.status === 'unauthenticated';
    alertText(response.status === 'unauthenticated' ? $('sign-in-error') : $('list-error'), `The API refused the call: ${refusalText(response)}`);
    return;
  }
  alertText($('sign-in-error'), '');
  $('main').hidden = false;
  cursor = response.result.cursor;
  const items = response.result.proposals;
  $('as-of').replaceChildren(note(`As of ${response.result.as_of}. ${items.length} open proposals, the oldest first.`, 'caption'));
  cards($('list'), listCards(items), 'No open proposal.');
  $('open-buttons').replaceChildren(...items.slice(0, 100).map((item) => {
    const button = make('mu-button', fill('button', { label: clip(`Open ${item.function}`, 40) }));
    button.dataset.snapshot = item.snapshot_ref;
    button.addEventListener('mu-press', () => openProposal(item.snapshot_ref));
    return button;
  }));
  show('list');
}

async function readProposal(ref) {
  const response = await call(token, 'proposal', { snapshot_ref: ref });
  if (response.status === 'ok') cursor = response.result.cursor;
  return response;
}

function renderEvidence(result) {
  put($('evidence'), proposalCard(result.proposal, result.snapshot));
  put($('source'), sourceCard(result.source));
  $('bundle').replaceChildren(...Object.keys(result.bundle ?? {}).sort().flatMap((name) => {
    const part = excerpt(result.bundle[name], MAX_TEXT);
    return [note(name, 'strong'), note(part.text), part.omitted ? note(`The page omits ${part.omitted} more characters.`, 'caption') : null]
      .filter(Boolean);
  }));
  cards($('reports'), result.reports.map(reportCard), 'No report. A verdict needs a qualification report.');
  cards($('verdicts'), verdictCards(result.decisions), 'No verdict.');
}

async function openProposal(ref) {
  const response = await readProposal(ref);
  if (response.status !== 'ok') {
    alertText($('list-error'), `The API refused the call: ${refusalText(response)}`);
    return;
  }
  current = response.result;
  kept = '';
  renderEvidence(current);
  bindVerdict(current);
  show('detail');
}

function bindVerdict(result) {
  const reports = result.reports;
  const report = reports.length ? reports[reports.length - 1].report_ref : null;
  const form = $('reason');
  form.action = { tool: 'dashboard.keep_reason', label: 'Keep reason', title: 'Verdict reason', editable: { reason: 'Reason' } };
  put(form, fill('editable-form', { title: 'Verdict reason', submit_label: 'Keep reason', fields: [
    fill('field', { name: 'reason', label: 'Reason', value: '', hint: 'The accept and reject buttons send the reason that you keep here.' }),
  ] }, PAGE_ACTOR));
  for (const disposition of ['accept', 'reject']) {
    const button = $(disposition);
    const label = disposition === 'accept' ? 'Accept' : 'Reject';
    const description = `${label} snapshot ${shortRef(result.proposal.snapshot_ref)}`;
    if (!report) {
      button.action = null;
      button.fill = fill('action-button', { label });
      continue;
    }
    // The confirm step: the button sends decide only after the person confirms.
    button.action = {
      tool: 'entelos.decide', label, description,
      arguments: { disposition, report_ref: report },
      confirm: { message: `${description} with the kept reason? The API keeps each verdict.`, label: `Confirm ${disposition}` },
    };
    button.fill = fill('action-button', { label, description });
  }
}

// The form keeps the reason in the page. It does not call the API.
$('reason').addEventListener('mu-action', (event) => {
  const { id, arguments: args } = event.detail;
  const reason = (args.reason ?? '').trim();
  kept = reason;
  event.target.receipt(reason
    ? { id, ok: true, message: 'The page keeps this reason for the verdict.' }
    : { id, ok: false, message: 'Write a reason.' });
});

// A verdict button sends decide. The button shows done only after the API gives the verdict record.
for (const disposition of ['accept', 'reject']) {
  $(disposition).addEventListener('mu-action', async (event) => {
    const { id, arguments: args } = event.detail;
    const button = event.target;
    if (!kept) {
      button.receipt({ id, ok: false, message: 'Keep a reason first.' });
      return;
    }
    const response = await call(token, 'decide', {
      report_ref: args.report_ref, reason: kept, disposition: args.disposition, expected_revision: cursor, request_key: id,
    });
    if (response.status !== 'ok') {
      button.receipt({ id, ok: false, message: `The API refused the verdict: ${refusalText(response)}` });
      return;
    }
    cursor = response.result.cursor;
    // The receipt reference is the verdict record.
    button.receipt({ id, ok: true, message: 'The API recorded the verdict.', reference: response.result.result.decision_ref });
    // Show the new verdict record. The buttons keep their state.
    const again = await readProposal(current.proposal.snapshot_ref);
    if (again.status === 'ok') {
      current = again.result;
      renderEvidence(current);
    }
  });
}

$('back').fill = fill('button', { label: 'Back to the list' });
$('back').addEventListener('mu-press', loadList);
show('none');
