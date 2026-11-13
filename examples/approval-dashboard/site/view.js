// Pure functions that turn API results into mugham fills. They make no DOM node and no API call.
// The kit shows each value as text, never as markup.

export const KIT_VERSION = '0.1.0';

export const fill = (component, data) => ({ component, version: KIT_VERSION, data });
export const fact = (label, value) => fill('fact', { label: clip(label, 40), value: clip(String(value), 200) });

// clip keeps the first max code points. The contracts count code points.
export function clip(value, max) {
  const points = [...String(value)];
  return points.length <= max ? points.join('') : points.slice(0, max - 1).join('') + '…';
}

// A long text keeps its first max code points and says how much it omits.
export function excerpt(value, max) {
  const points = [...String(value)];
  if (points.length <= max) return { text: points.join(''), omitted: 0 };
  return { text: points.slice(0, max).join(''), omitted: points.length - max };
}

export function shortRef(ref) {
  return typeof ref === 'string' && ref.startsWith('sha256:') ? ref.slice(0, 19) + '…' : String(ref ?? '');
}

// The wait time in days, hours and minutes. A negative value shows that the clock went back.
export function waitText(seconds) {
  if (typeof seconds !== 'number' || !Number.isInteger(seconds)) return 'unknown';
  if (seconds < 0) return `clock went back by ${-seconds} s`;
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days) return `${days} d ${hours} h`;
  if (hours) return `${hours} h ${minutes} min`;
  return `${minutes} min`;
}

// The verdict kind of a stored verdict record. A record of format entelos.policy/1 is a policy verdict.
// Each other verdict of the approval API is a decide record, so it is a human verdict.
export function verdictKind(record) {
  return record && record.format === 'entelos.policy/1' ? 'policy' : 'human';
}

// The verdict status of a proposal item: what verdict the proposal has or needs.
// A stale report or a superseded snapshot is "stale", because a new qualification or proposal is necessary.
export function verdictStatus(item) {
  if (item.state === 'superseded') return { kind: 'stale', text: 'Stale: a newer proposal replaces this one' };
  if (item.refusal === 'stale_qualification') return { kind: 'stale', text: 'Stale: the report needs a new qualification' };
  switch (item.state) {
    case 'accepted': return { kind: 'human', text: 'Accepted' };
    case 'rejected': return { kind: 'human', text: 'Rejected' };
    case 'awaiting_qualification': return { kind: 'none', text: 'Waits for a qualification report' };
    case 'awaiting_verdict': return { kind: 'human', text: 'Waits for a human verdict' };
    case 'not_approvable': return { kind: 'none', text: `Not approvable: ${item.refusal}` };
  }
  return { kind: 'none', text: `Unknown state: ${item.state}` };
}

const date = (time) => (typeof time === 'string' && /^\d{4}-\d{2}-\d{2}(T|$)/.test(time) ? time.slice(0, 10) : undefined);

function dated(data, time, label) {
  const day = date(time);
  return day ? { ...data, date: day, date_label: label } : data;
}

// One card of the list. The card shows the wait time and the verdict status as facts.
export function listCard(item) {
  const status = verdictStatus(item);
  return fill('card', dated({
    title: clip(item.function, 120),
    summary: clip(item.reason ?? '', 1000),
    facts: [
      fact('Wait', waitText(item.wait_seconds)),
      fact('Verdict', status.text),
      fact('State', item.state),
      fact('Reports', String(item.report_refs?.length ?? 0)),
      fact('Verdicts', String(item.decision_refs?.length ?? 0)),
      fact('Snapshot', shortRef(item.snapshot_ref)),
    ],
  }, item.submitted_at, 'Submitted'));
}

export function listGrid(items) {
  return fill('card-grid', { label: 'Open proposals', cards: items.slice(0, 100).map(listCard) });
}

// The evidence card of one proposal.
export function proposalCard(item, snapshot) {
  return fill('card', dated({
    title: clip(item.function, 120),
    summary: clip(item.reason ?? '', 1000),
    facts: [
      fact('Wait', waitText(item.wait_seconds)),
      fact('Verdict', verdictStatus(item).text),
      fact('Producer', item.producer ?? snapshot?.producer ?? ''),
      fact('Interval', `${item.from} to ${item.until}`),
      fact('Scope', item.scope),
      fact('Snapshot', shortRef(item.snapshot_ref)),
    ],
  }, item.submitted_at, 'Submitted'));
}

export function sourceCard(source) {
  return fill('card', {
    title: 'Source',
    summary: clip(source?.text ?? '', 1000),
    facts: [fact('Reason', source?.reason ?? ''), fact('Interval', `${source?.from} to ${source?.until}`)],
  });
}

export function reportCard(entry, index) {
  const report = entry.report ?? {};
  const cases = Array.isArray(report.outcome?.cases) ? report.outcome.cases : [];
  return fill('card', {
    title: `Report ${index + 1}: ${report.status ?? 'unknown'}`,
    facts: [
      fact('Status', report.status ?? 'unknown'),
      fact('Report refusal', entry.refusal || 'none'),
      fact('Runner', report.runner ?? 'unknown'),
      fact('Cases passed', `${cases.filter((c) => c.passed === true).length} of ${cases.length}`),
      fact('Report', shortRef(entry.report_ref)),
    ],
  });
}

// A human verdict and a policy verdict have different titles and facts. A policy verdict never counts as a human verdict.
export function verdictCard(record, ref) {
  if (verdictKind(record) === 'policy') {
    return fill('card', dated({
      title: `Policy verdict: ${record.outcome}`,
      summary: 'A policy wrote this verdict. It does not count as a human verdict.',
      facts: [fact('Kind', 'policy'), fact('Outcome', record.outcome), fact('Reason code', record.reason), fact('Verdict', shortRef(ref))],
    }, record.date, 'Date'));
  }
  return fill('card', dated({
    title: `Human verdict: ${record.disposition ?? 'accept'}`,
    summary: clip(record.reason ?? '', 1000),
    facts: [fact('Kind', 'human'), fact('Actor', record.actor), fact('Disposition', record.disposition ?? 'accept'),
      fact('Decided at', record.decided_at ?? 'unknown'), fact('Verdict', shortRef(ref))],
  }, record.decided_at, 'Decided'));
}

export function verdictGrid(entries) {
  return fill('card-grid', { label: 'Verdicts', cards: entries.slice(0, 100).map((e) => verdictCard(e.decision, e.decision_ref)) });
}

// The text of an API refusal. The receipt message of the kit has at most 200 characters.
export function refusalText(response) {
  const d = response?.diagnostic ?? {};
  return clip(`${d.code ?? response?.status ?? 'error'} (${d.stage ?? 'unknown stage'}): ${d.message ?? 'no message'}`, 200);
}
