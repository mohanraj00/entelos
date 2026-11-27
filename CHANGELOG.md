# Changelog

This file lists the changes for each snapshot tag. A tag is a stable snapshot to cite and copy. It is not a package release. Up to v0.3.0, a tag does not promise a stable API or record format. From v0.4.0, [STABILITY.md](docs/STABILITY.md) gives the promise for the surfaces that it names.

## Unreleased

### Reference code

- A published record for a suspension (#211): [ACTIVATION.md](src/protocol/activation/ACTIVATION.md#suspension-record), a schema and 25 hand-written vector cases. The new record kind `suspension` names the scope, the subject, the interval, the targets, the actor and the event reference of one `suspend` event. A Host with a signing key writes and signs the record in the `suspend` write. `VerifySuspension` checks a signed suspension record offline. `VerifyActivation` refuses a suspension record with `unexpected_record_kind`, and the signature covers the record kind.
  - The publisher now also gets the suspension record, before the commit of a `suspend` write. A refusal gives `publish_refused` and stores nothing, and a retry of the same request gives the same bytes. A Host with a publisher refuses a `suspend` with `publish_unsigned` or `publish_unsupported` in the cases where it refuses an `activate`. A publisher that accepts only activation records must refuse a record with the `record_kind` `suspension`.
  - Effect on a store: the new record kind is `suspension`, and the new state field is `suspensions`. A signing Host writes two more records for each `suspend` write: the suspension record and its signed record. A store gets the field at its first signed suspension. A Host without a signing key writes the same bytes as before. No migration is necessary.
  - The signed record field `record_kind` accepts the new value `suspension`. A verifier of an earlier version refuses such a signed record with `invalid_signed_record`, rule `schema`.
  - The wire protocol and the pinned stable surfaces do not change. The sidecar still has no publisher setting.
  - Limits: a suspension before the signing key has no suspension record, and the record does not copy the `reason` of the event.
- The Go Host writes the policy verdict of [POLICY.md](src/protocol/policy/POLICY.md) (#210). The Host-only write `policy_decide` calls the one policy function of the new Host context field `policy_functions`, and writes a `policy_verdict` record with an `invoke` receipt. An accept or a refuse also writes a decision record of the kind `policy`. The Host-only read `policy_replay` replays the verdict. The Host context field `requirements` gives the verdict requirement of each head; the default is `human`. If the requirement is `human`, `activate` refuses an activation that only a policy verdict supports with the new code `human_verdict_required`. The qualification report gets the optional field `change_class` from the new runner field `Verdict.ChangeClass`. The wire protocol does not change.
  - Effect on a store: the records keep their bytes. A Host context without `policy_functions` and `requirements` keeps the earlier behavior.
- A time limit for the publisher call, and a recovery of a stale lock file of the root ref (#212). See [ACTIVATION.md](src/protocol/activation/ACTIVATION.md#stale-lock-of-the-root-ref).
  - The Go interface `Publisher` changes: `Publish` now takes a `context.Context` as its first parameter. The context has the deadline of the call. The default limit is 10 seconds, and the new function `Host.SetPublishTimeout` sets another limit. At the limit, the result is the new code `publish_timeout` with the stage `publish`, the store commits nothing, and the store lock is free. To update a publisher, add the parameter and stop the work when the context is done.
  - The new function `GitStore.RecoverRootLock` and the new command `entelos-unlock --repo <store.git> [--check]` examine the lock file of the root ref and remove a stale one. A `git` process that stops with SIGKILL during a write leaves this file, and each later write then fails with `store_error`. The recovery refuses with the new code `update_in_progress` while an update is in progress. It does not change the root ref.
  - Effect on a store: the Git store makes the empty file `lifecycle-root-update.flock` in the repository at its first write. Each update of the root ref takes a shared `flock` lock on it. The records and the root do not change, and no migration is necessary. On a file system that does not support `flock`, a write takes no lock on the marker file, and the recovery refuses with `store_error`.
  - Limits: the recovery sees only a Host of this version or later on the same operating system kernel, and it needs a Unix system. The Host does not stop a publisher that ignores its context.

### Decisions

- The maintainer accepted [Decision 0004](docs/decisions/0004-evidence-number-domain.md) on 2026-11-27, after the Host change merged in PR #218 (#208). The decision text does not change (#222).

### Contents

- The approval dashboard moves from the mugham UI kit `v0.3.0` to `v0.7.0`. It sends the action key of the kit as the `request_key` of `decide`, so a retry of one verdict records it only once.

## v0.7.0 (2026-11-26)

This snapshot checks the evidence of a content proposal at `propose` (#207) and uses one fixed-point number domain for evidence admission (#208). It adds the finding import contract for one nookku finding record (#112). A new test runs a step result through the CEL runtime (#209). A CI workflow runs the kind seam test of the agent step (#215). The wire protocol, the record format and the pinned stable surfaces do not change.

### CI

- The [Kind seam](.github/workflows/kind-seam.yml) workflow runs the three agent step seam tests on one runner (#215). It keeps the failure logs for 7 days. A failure does not block a merge.

### Reference code

- Evidence uses safe JSON integers on all admission paths (#208). Producers write judgment `p` in parts per million and box coordinates in thousandths of a point. Add the unit marker for either field and use strings for any exact decimals in attached objects. Recompute the manifest evidence digest after conversion. Admission refuses earlier fractional, out-of-range or unmarked box and judgment data with `invalid_evidence`, rule `schema`. The format name stays `entelos.evidence/1`. See the [number rules](src/protocol/evidence/EVIDENCE.md#numbers).
  - Effect on a store with old snapshots: the records keep their bytes, and no migration is necessary. A content snapshot that the #207 rules passed can hold boxes or judgments with no unit marker. For such a snapshot, an accept `decide` and `activate` now refuse with `invalid_evidence`, rule `schema`. A reject `decide` still works. An existing decision or activation event does not change. To continue, convert the evidence and propose a new snapshot with the converted document.
- A content proposal checks its evidence at `propose` (#207). [CONTENT.md](src/protocol/content/CONTENT.md#evidence) defines the digest that the manifest field `evidence.digest` identifies: SHA-256 of the canonical text of the evidence document. If the manifest names evidence, the request must attach that document. Else `propose` refuses with `evidence_missing` or `evidence_mismatch`. An attached document must pass the 14 rules of [EVIDENCE.md](src/protocol/evidence/EVIDENCE.md#rules), else `propose` refuses with `invalid_evidence` and the rule. Each refusal has the stage `admission` and writes nothing. A document without a manifest reference is still permitted. A CEL bundle proposal keeps the behavior of approval API 1.2.
  - Effect on a store with old snapshots: the records keep their bytes, and no migration is necessary. A content snapshot of an earlier Host can have a reference with no document, a document with another digest, or a document that the rules refuse. For such a snapshot, an accept `decide` and `activate` now refuse with the same codes. A reject `decide` still works. An existing decision or activation event does not change, and `resolve` and a replay give the same answers. To continue, propose a new snapshot with the named document.
  - The approval `state` of such a snapshot can still be `awaiting_verdict`, because the `refusal` values of approval API 1.2 are fixed.
  - The lifecycle vectors of the content format change: the manifest `second` names the digest of a vector document, so its `ref` changes. The new case `evidence-at-propose` covers each new code.
- A lifecycle test qualifies and invokes a typed CEL gate on step results, covering acceptance, refusal and a failed run with no decision inputs (#209).
- The finding import contract (#112): [FINDING.md](src/lifecycle-core/FINDING.md) and `finding.go` in the Go lifecycle core. The Host-only write `import_finding` reads one nookku finding record of record version 1.0 and stores a candidate case with its origin and the SHA-256 of the input text. The reader ignores an unknown field, accepts a newer minor version and refuses an unknown major version. The Host-only write `decide_case` records one decision of a reviewer on a candidate. The `plan` request takes the Host-only field `imports`, and it refuses a candidate with no accept decision with `case_not_approved`. An import gives no passed flag and activates nothing, and a candidate is not a case before a plan imports it.
  - Effect on a store: the new record kinds are `case_candidate` and `case_decision`, and the new state field is `case_candidates`. A store gets the field at its first import. A plan with no import keeps its bytes. No migration is necessary.
  - The wire protocol, the record format and the pinned stable surfaces do not change. Each new declaration is not exported.
  - `make verify` has the new check `nookku-seam`. It installs nookku 0.6.1 from PyPI in a virtual environment, and [`tests/nookku-seam/requirements.txt`](tests/nookku-seam/requirements.txt) pins the wheel by its SHA-256. The released tool runs the invented library example with a local agent and exports three finding records. The Host reads these records and nine changed copies.
  - Limits: no shipped runner reads the case format `entelos.finding-case/1`, a case decision has no signed record, and the Host does not keep the record text.

### Upgrade steps

These steps upgrade a Host and a store from v0.6.0.

1. The record format stays `entelos.record/1`, and the wire protocol versions 1.0, 1.1 and 1.2 do not change. A v0.6.0 store needs no migration command. No test runs a v0.6.0 store on the v0.7.0 Host, so make a copy of the bare repository before the first write.
2. If you produce evidence, write the judgment `p` in parts per million (#208). Write the box coordinates in thousandths of a point. Add the unit marker for each field. Use strings for exact decimals in attached objects. After the conversion, compute the manifest evidence digest again. An accept `decide` and `activate` refuse a content snapshot with an unmarked box or judgment, with `invalid_evidence` and rule `schema`. To continue, convert the evidence. Then propose a new snapshot. [Decision 0004](docs/decisions/0004-evidence-number-domain.md) has the status `proposed`.
3. If a content proposal names evidence in the manifest field `evidence.digest`, attach that document to the `propose` request (#207). The digest is the SHA-256 of the canonical text of the document. Else `propose` refuses with `evidence_missing`, `evidence_mismatch` or `invalid_evidence`. A content snapshot of an earlier Host gets the same refusals at an accept `decide` and at `activate`. To continue, propose a new snapshot with the named document.
4. If a reader shows the approval `state`, expect `awaiting_verdict` for a snapshot that the evidence check refuses (#207). The `refusal` values of approval API 1.2 do not change.
5. The changes to the Go lifecycle core change the lifecycle build identity. Keep the v0.6.0 build with a store for exact replay.
6. `make verify` has the check `nookku-seam` (#112). It installs nookku 0.6.1 from PyPI in a virtual environment, and [`tests/nookku-seam/requirements.txt`](tests/nookku-seam/requirements.txt) pins the wheel by its SHA-256. To import a finding, call the Host-only write `import_finding`. Record the decision of a reviewer with `decide_case`. Then send `plan` with the field `imports`.

### Compatibility and limits

- [STABILITY.md](docs/STABILITY.md) gives the promise of v0.4.0, and v0.7.0 keeps it. The pinned surfaces do not change. The evidence format `entelos.evidence/1` stays experimental. The new declarations of the finding import are not exported.
- Decision 0004 has the status `proposed`. Protocol 1.2 keeps its number domain.
- An existing decision or activation event does not change, and `resolve` and a replay give the same answers. A reject `decide` still works on a snapshot that the evidence check refuses.
- No shipped runner reads the case format `entelos.finding-case/1`, a case decision has no signed record, and the Host does not keep the record text. The wire protocol does not have the writes `import_finding` and `decide_case`.
- #209 and #215 remove two limits of v0.6.0. A test runs a step result through the CEL runtime with a fixture step runner. The Kind seam workflow runs the kind test. A failure of that workflow does not block a merge.
- The limits of v0.6.0 that this list does not change stay. The policy verdict is a proposed spec that no Host writes. The sidecar wire protocol does not accept `candidate` and `applicability`. The callable profile is string and bool input and bool output.

## v0.6.0 (2026-11-20)

This snapshot adds the step outcome contract and an optional adapter that runs an agent step as a koodam Agent run. The decision event export now continues past a verdict with no per-fact marks. The approval dashboard uses mugham v0.3.0, and the project uses the shared gates at a new pin. The wire protocol, the record format and the stable Go API do not change.

### Reference code

- The decision event export continues past a verdict with no blind marks (#197). A verdict that cites facts and has no `fact_reviews` in its record is an unmarked verdict: a human verdict from a client on protocol 1.0 or 1.1, or a verdict of an actor with no kind. v0.5.0 refused each `decision_events` page with such a verdict. Now the facts of an unmarked verdict keep their positions and give no events, so the positions have a gap. The Host supplies no mark and no kind. [EVENTS.md](src/protocol/events/EVENTS.md#unmarked-verdicts) has the rule and the reasons. The format `entelos.decision-event/1`, the 1.2 schemas, the cursor and the stream namespace do not change. A decision with marks that are not complete or not valid still gives `unexportable_decision`.
  - Page rule: `limit` counts events, and a gap position does not count. A page with `limit` events ends at its last event, as on v0.5.0. A page with fewer events reached the stream end, and its `next_cursor` is the cursor at the last position of the stream. Thus no page before the stream end is empty. `next_cursor` is after the last event only if the stream ends in a gap. For a stream with no unmarked verdict, each page is the page that v0.5.0 gave.
  - Effect on a consumer: a reader that checks for contiguous positions must accept a jump. A gap means the omitted facts of an unmarked verdict, and it never means a lost event. The reader can count the omitted facts: the position of `next_cursor`, minus the position of the request cursor, minus the count of events. An empty page still means that the stream has no later event at this time. Its `next_cursor` can be after the request cursor, if the stream ends in a gap.
  - Recovery for a reader that stopped on v0.5.0: upgrade the Host, then send the saved cursor again. The page starts with the first event after the gap, with no new stream, no duplicate and no lost event. Each event has the position that v0.5.0 counted for it. No store migration is necessary.
  - To get an event for each human verdict, send `fact_reviews` through protocol 1.2 for each verdict that cites facts.
- The step outcome contract (#113): [STEP.md](src/lifecycle-core/STEP.md) and `step.go` in the Go lifecycle core. A workflow agent step gives its inputs by digest. The result has a run identity, the digests of the output and the trace, and one of the statuses `completed`, `failed` and `budget_stopped`. `StepDecisionInputs` gives `invoke` inputs only for a completed result, so the receipt binds the run, and a `replay` uses the bound output with no new run. A failed run or a budget stop gives the fault `step_failed` or `step_budget_stopped` and no inputs. The Host and the wire protocol do not change. Each declaration is experimental.
- The optional adapter [examples/agent-step-koodam](examples/agent-step-koodam/README.md) (#113) runs a step as one koodam Agent run through `kubectl`. [CONTRACT.md](examples/agent-step-koodam/CONTRACT.md) gives its clauses. The core has no Kubernetes dependency, and the adapter module has no Kubernetes library. A seam test runs on a local kind cluster with the koodam release v0.4.0, the first release with the run identity and the output digest. The test pins the tag, the chart and the koodam image by digest, and it has no stand-in for a koodam field. No CI job runs the kind test.

### Contents

- The approval dashboard (#198) moves from the mugham UI kit `v0.2.1` to `v0.3.0`
  (commit `f61f5d4`). The contract format stays 2 and the catalog contracts stay
  at 0.1.0, so the page code does not change. The page does not use the new
  components of the kit. `run.py` accepts only the pinned commit.

### Project checks

- The project moves the shared gates to `mohanraj00/.github` commit `4d0656c141e5a5969bd72a5fe10636cc8dd9d524` (#201). `docs/STANDARD.md`, the Dependabot file and `tools/claude_review.py` are copies of that release, and the Claude review workflow is the release template with the project pin. A Claude review request now needs the request text on the first line only, so a later line such as `Head: <short SHA>` is allowed. The local `tools/cross_review.py` is removed. The agy fallback is now `gates/cross_review.py` of the shared repository, run from a checkout at the pin. [REVIEW.md](docs/REVIEW.md) has the command. These files are for contributors. A consumer of the reference does not need them.

### Upgrade steps

These steps upgrade a Host and a store from v0.5.0.

1. The record format stays `entelos.record/1`, and the wire protocol versions 1.0, 1.1 and 1.2 do not change. A v0.5.0 store needs no migration command. No test runs a v0.5.0 store on the v0.6.0 Host, so make a copy of the bare repository before the first write.
2. If a reader of `decision_events` checks that the positions are contiguous, change it to accept a jump (#197). A jump means the omitted facts of an unmarked verdict. It never means a lost event. A reader that stopped with `unexportable_decision` on v0.5.0 sends its saved cursor to the v0.6.0 Host and continues. To get an event for each human verdict, send `fact_reviews` through protocol 1.2.
3. The Go lifecycle core has 11 new experimental declarations for the step contract (#113). The new file `step.go` changes the lifecycle build identity, as each core change does. Keep the v0.5.0 build with a store for exact replay. No Host path calls the new code.
4. To use the koodam adapter, build `examples/agent-step-koodam`. It is a Go module of its own with the standard library only. It needs `kubectl` and a koodam cluster at release v0.4.0 or later. The core needs neither.
5. A contributor who runs the agy fallback checks out `mohanraj00/.github` at the pin and runs its `gates/cross_review.py` from the root of this project. `tools/cross_review.py` no longer exists.
6. The approval dashboard accepts only the mugham commit in `mugham.json`. `run.py` refuses another commit.

### Compatibility and limits

- [STABILITY.md](docs/STABILITY.md) gives the promise of v0.4.0, and v0.6.0 keeps it. The pinned surfaces do not change. The 11 new Go declarations are experimental.
- #197 changes the output of a stable read. The read `decision_events` of protocol 1.2 now gives a page with a jump in the positions for an unmarked verdict, where v0.5.0 refused the page. The format `entelos.decision-event/1`, the 1.2 schemas, the cursor and the stream namespace are the same. A stream with no unmarked verdict gives the same pages as v0.5.0. The owner merged #204 as a level 1 change.
- A gap gives the count of the omitted facts. It does not give the count of the verdicts, the actor or the disposition. No Host setting refuses an unmarked verdict at `decide`. The Host supplies no mark and no kind.
- No test starts a v0.5.0 Host. The resume rule for a reader comes from the v0.5.0 read rules, not from a run of that Host.
- The kind evidence for #113 is one local run from a clean clone of `49a6000`, on a local kind cluster, on 2026-11-20. It is not a CI run, and no CI job runs the kind test. The comment on #113 has the record. The run used kind v0.33.0 and the node image v1.37.0, so the three koodam fields are shown on one node image only. The three tests passed: a completed run, a failed run and a budget stop. Three earlier runs of the same commit failed before a koodam step: two `kubeadm init` timeouts under a high host load, and one name lookup error for the container registry. They show nothing about the change.
- The first `make verify` run of that commit failed 2 checks under the same host load. The record of the two names is lost, because the work folder was removed before they were read. A second run passed 33 checks with 0 failures.
- The step contract trusts the step runner for the status and the run identity. The gate of the tests is a frozen Go function, and the qualification runner is a fixture. No test runs a step result through the CEL runtime. The Host has no step operation, so a workflow that sends other inputs to `invoke` gets no check from the contract. The trace of the adapter is the Agent spec and status, without the model calls.
- The CI run of the merge commit of #204 (`d4eb165`, run 44520665146) failed on `ubuntu-24.04` with 21 checks passed and 2 failed. The names of the 2 checks were not read for this note. The run of the same PR head before the merge passed on both runners, and run 44662780478 on `ca81eee` passed on both runners.
- No new full security review of the Go path covers v0.6.0. Each PR had a review of its own diff. Two security reviews of #204 found one defect each in earlier methods, and the merged head has no finding. The Codex security review of #203 found no issues.
- The limits of v0.5.0 that this list does not change stay: the policy verdict is a proposed spec that no Host writes, the sidecar wire protocol does not accept `candidate` and `applicability`, and the callable profile is string and bool input and bool output.


## v0.5.0 (2026-11-18)

This snapshot adds content proposals, signed approval and activation records, supersession, verdict binding and verdict kinds, the evidence read, the decision event export and approval API 1.2. It adds the Host part of the profile promotion. The policy verdict is still a proposed spec that no Host writes.

### Reference code

- The Go lifecycle Host exports decision events (#158, part 2). The protocol 1.2 read `decision_events` gives one event for each cited fact of each verdict after the stream start, in store order. Each event has the actor kind, a blind mark and the correction, if there is one. The read needs the new role `event_reader`, which permits no other operation. The optional protocol 1.2 field `decide.fact_reviews` records a blind mark and an optional corrected value for each cited fact. The first Host write saves the stream start in `state.decision_stream`, and `Host.StartDecisionEvents` saves it before a write. The example [decision-events](examples/decision-events/README.md) reads the pages, and it saves and resumes its cursor. The new Go declarations `CheckDecisionEvent` and `Host.StartDecisionEvents` are experimental.
- The Go lifecycle Host applies the evidence admission rules (#104). The Host-only read `evidence` applies the 14 rules of [EVIDENCE.md](src/protocol/evidence/EVIDENCE.md) to the retained document of a snapshot. It gives the result and the four policy inputs `evidence_human_reviewed`, `evidence_model_reviewed`, `evidence_all_automatic` and `evidence_corrected`. It writes nothing. `propose` does not refuse a document that a rule refuses, so approval API 1.2 does not change. The 97 evidence vectors run against the Host rules. A seam test maps a groundgate 0.16.0 `facts@2` export and its store records to the evidence format. `verify.py` installs the release by its hashes, and the evidence checker and the Host evidence read accept the mapped document with the expected policy inputs.
- The verdict binding (#127): [VERDICT.md](src/protocol/verdict/VERDICT.md). A verdict judges the candidate reference of its snapshot. The decision record does not change. A verdict is stale when the head of its subject has other content. `decide` and `activate` refuse it with `stale_verdict`. The read `approval` answers if a candidate reference has a current approval for a subject. The Go lifecycle Host applies it, with 6 minimum vector cases.
- Verdict kinds (#128): [KINDS.md](src/protocol/verdict/KINDS.md). The kinds are `human`, `model` and `policy`. The Host context field `actor_kinds` gives the kind of each actor, and `verdict_policy` gives the count of each kind that `activate` requires. A verdict counts only for its own kind. The new refusal code is `verdicts_required`. The Go lifecycle Host applies it: `decide` writes `kind` in the decision record, `activate` checks the verdict policy, and the `approval` read gives `counts` and `required`. Without the two fields, the record bytes and the behavior do not change. 24 hand-written vector cases cover each combination of the KINDS.md table.
- A proposed spec for a profile promotion (#152): [PROMOTION.md](src/protocol/promotion/PROMOTION.md), a schema and hand-written vectors. A promotion is a content proposal whose manifest names a profile version and its measure report. The qualification runner gives a measure summary `entelos.measure/1` with each route bound as an integer in basis points. Nine rules bind the summary to the manifest. The Host builds the new policy inputs `measure_max_bound_bp` and `measure_gold_digest` of [POLICY.md](src/protocol/policy/POLICY.md), so a policy can accept or refuse a promotion. The limit is in the policy, so a change to a limit needs a person.
- The Go lifecycle Host applies the profile promotion rules (#152). The optional Host context field `promotion_collections` names the promotion collections. The new optional field `Verdict.Measure` gives the measure summary of the runner, and `qualify` stores it as the report field `measure`. A report without a summary keeps its bytes. The Host-only reviewer read `promotion` applies the nine rules of PROMOTION.md to a stored report and its snapshot. It gives the first rule that fails, or the inputs `measure_max_bound_bp` and `measure_gold_digest`. It writes nothing. The 24 rule checks of the vectors run against the Go rules. No Host writes a policy verdict yet. The new field `Verdict.Measure` is experimental ([STABILITY.md](docs/STABILITY.md#experimental-surfaces)). A caller that writes `Verdict` without field names must name the fields.
- A generic activation record (#105): [ACTIVATION.md](src/protocol/activation/ACTIVATION.md), a schema and 31 hand-written vector cases. The record kind `activation` names the candidate, the base, the receipts, the actors, the partition, the interval and the applicability values of one `enable` event. A Host with a signing key writes and signs it in the `activate` write, and `state.activations` maps the event to it. Without a key, the Host behavior does not change. `lifecycle.VerifyActivation` checks a signed activation record offline with the signed record checks and 8 rules. The optional `Publisher` that `Host.SetPublisher` installs gets the record bytes before the commit, while the store holds the lock of the root update. A write that loses the compare-and-swap does not publish. A refusal is `publish_refused`, and the write stores nothing. A Host with a publisher and no signing key refuses `activate` with `publish_unsigned`. A Host with a publisher and a store of another package refuses it with `publish_unsupported`. The wire protocol and the sidecar have no publisher setting.
- The Host can sign each approval record and each activation record (#155). The embedding host installs one Ed25519 signing key with `Host.SetSigner`, or gives `--signing-key` to `lifecycle-core` or `entelos-sidecar`. Each write of a `decision` record or an `event` record then also stores a signed record of the kind `signed`, and `state.signatures` maps the record to it. Without a key, the Host behavior does not change. With a key, the Host refuses a `decision` or `event` record of more than 512 KiB with `record_too_large`, so each signed record fits in one protocol frame. It checks each signed record with the verifier before the write. The spec is [SIGNED.md](src/protocol/signed/SIGNED.md). It defines the signed bytes, the key identifier, the public key set `entelos.keyset/1` with the statuses `active`, `retired` and `revoked`, and the order of the checks.
- A new activation can supersede an earlier one (#129). `activate` has the optional field `supersedes`, and the activation record names the superseded events. A superseded update is never selected at a date on or after the start of its successor, also after the successor ends or is suspended. A `targets` mask still applies only in the interval of the new event. A replay of an old answer uses the old update, as before. A worker that still has a superseded update gives `activation_superseded`, in each stale mode. The spec is [SUPERSESSION.md](src/protocol/supersession/SUPERSESSION.md), with vectors for a chain of three updates. The JSONL Host, Go API and wire protocol 1.2 accept the field. Versions 1.0 and 1.1 keep their earlier schemas.
- The Host reads the content proposal `entelos.content/1` (#175). `propose` takes the Host field `candidate`, with the kind `cel-bundle` or `content`. A content head has the subject `content:<collection>/<partition>` in the `function` field of its records, and the embedding host installs the subject in `functions`. The snapshot keeps the exact manifest text, and a passed report must name its SHA-256. A changed manifest is a new candidate and needs a new qualification. `resolve` gives the selected candidate reference, also at an earlier cursor. `load`, `invoke`, `describe` and `ingest` refuse a content snapshot with `unsupported_candidate_kind`, and the CEL runner refuses to qualify one. A CEL function name that starts with `content:` is refused. Lifecycle vectors with a fixture runner go from `propose` to `activate` and to a read at an earlier root. [Decision 0003](docs/decisions/0003-content-kind.md) (proposed) records the kind model. The sidecar wire protocol does not accept the field yet.
- A content item can declare typed applicability values (#106): the optional field `applicability`, with 1 to 16 names and 1 to 8 levels for each name. The rules `applicability_count` and `applicability_name` check them. The `activate` request confirms the values of the snapshot items with the Host field `applicability`, and a mismatch is `applicability_mismatch`. The event record and the `resolve` selection name the confirmed values. A manifest, a snapshot and an event with no values keep their bytes. The sidecar wire protocol does not accept the field yet.
- `lifecycle.VerifySigned` and the new command `entelos-verify` check a signed record file against a key set file with no call to entelos. A record of an unknown major version fails closed, also with a valid signature. The new command `entelos-keygen` makes a signing key file. The module still uses only the Go standard library.
- The vectors in `src/protocol/signed/vectors/` cover a valid record, a changed record, an unknown key, a revoked key and a retired key. `check_spec.py` checks them with a separate Ed25519 implementation in Python, and `TestSignedVectors` runs them on the Go verifier. The new declarations are experimental.
- Approval API 1.2 adds `proposals.limit` and an opaque `next_cursor` pinned to the first page's store revision and time (#161). Opens and closes during a traversal do not shift its pages. `propose` can retain a producer evidence document, and `proposal` returns it. Protocol 1.2 also adds the optional `activate.supersedes` array of event references (#180). Versions 1.0 and 1.1 still refuse that field with `invalid_request`. Session vectors cover version gating, the Host result and malformed values. Evidence admission and policy evaluation remain proposed. Requests without pagination keep their old result; the protocol 1.1 dashboard client is unchanged. The envelope adds 1.2 and unsupported-minor vectors now use 1.3. No exported Go declaration changes.

### Docs

- The README starts with the approval workflow for versioned content (#163). The new [content-update example](examples/content-update/README.md) embeds the Host with file digest and size checks, a failed qualification, human approval and a dated read. The rule demo follows it.
- [EVENTS.md](src/protocol/events/EVENTS.md) proposes `entelos.decision-event/1`, with a schema, hand-written vectors and an offline checker (#158, part 1). The export has one event for each fact, the actor kinds, a blind mark and cursors that a reader can resume. Part 2 adds the Host export (see above).

### Upgrade steps

These steps upgrade a Host and a store from v0.4.0.

1. The record format stays `entelos.record/1`, and the review index of v0.4.0 does not change. Thus a v0.4.0 store needs no migration command. The new state fields come with later writes. No test runs a v0.4.0 store on the v0.5.0 Host, so make a copy of the bare repository before the first write.
2. To use the page cursor, the evidence document, `activate.supersedes`, `decide.fact_reviews` or `decision_events`, send protocol `1.2`. The pinned vectors of protocol 1.0 and 1.1 keep their results. The first minor version that the Host refuses is now `1.3`, so a client test that expects a refusal of `1.2` must use `1.3`.
3. To read decision events, give a principal the role `event_reader` in the credentials file. This role permits no other operation, and the `reviewer` role does not permit the read. The credentials schema of v0.5.0 accepts the role.
4. Configure the decision event export before the first write of a v0.5.0 Host, because each Host write saves the stream start if it is absent. Earlier verdicts are not in the stream, and the start does not move.
   1. Give each actor that decides a kind in the Host context field `actor_kinds`.
   2. Change each human client to send `fact_reviews` in each `decide` that cites facts. A `model` or `policy` verdict needs no `fact_reviews`, because the Host records false marks from the kind of its actor.
   3. Optionally, call `Host.StartDecisionEvents` to save the start before the first write.

   Caution: Do not record a verdict without an actor kind, or a human verdict without `fact_reviews` for its facts, after the start. The read refuses each page that has such a verdict with `unexportable_decision` and gives no next cursor. Thus the read cannot go past that verdict, and v0.5.0 has no recovery step for it.
5. To sign records, install a key with `Host.SetSigner` or `--signing-key`. With a key, the Host refuses a `decision` or `event` record of more than 512 KiB with `record_too_large`. Keep the public key set for offline checks with `entelos-verify`.
6. If a Go runner writes a `Verdict` literal without field names, name the fields. `Verdict` has the new experimental field `Measure`.
7. `make verify` runs the groundgate seam test. It installs groundgate 0.16.0 from PyPI by the hashes in `tests/groundgate-seam/requirements.txt`, so it needs network access to PyPI. A mapping of an export of another groundgate version needs a new seam test and new hashes.

### Compatibility and limits

- [STABILITY.md](docs/STABILITY.md) gives the promise of v0.4.0, and v0.5.0 keeps it. The stable surfaces are the exported Go API that the pin lists, wire protocol 1.0, 1.1 and 1.2, and the record format `entelos.record/1`. v0.5.0 adds the protocol 1.2 files to the pinned surface. Each new Go declaration of v0.5.0 is experimental.
- The stable `Verdict` line of the pin changed, because the struct got the field `Measure`. STABILITY.md does not forbid an added field, and it lists the field as experimental. The owner merged this change in #195.
- `decide` and `activate` refuse a stale verdict with the new code `stale_verdict` on each protocol version (#127). With the Host context field `verdict_policy`, `activate` can refuse with `verdicts_required` (#128). Thus a protocol 1.0 or 1.1 request that v0.4.0 accepted can get a refusal. The pinned vectors keep their results, and the envelope schema permits each diagnostic code. As for the scope refusal of v0.4.0, the owner decides on this change. A client of protocol 1.0 or 1.1 must handle these codes.
- The approval version vectors moved the first refused minor version from `1.2` to `1.3`, as STABILITY.md permits. The credentials format `entelos.credentials/1` accepts the role `event_reader`.
- The policy verdict of POLICY.md is a proposed spec. No Host writes it. The promotion read gives the two measure inputs, but no Host evaluates a policy with them. `propose` does not refuse an evidence document that a rule refuses.
- The sidecar wire protocol does not accept the Host field `candidate` of `propose` or the field `applicability` of `activate`. The wire protocol and the sidecar have no publisher setting. The wire protocol does not have the reads `evidence` and `promotion`.
- The fixture runners of the content and promotion tests are not runners of an application. No real profile or measure report ran.
- A content hash identifies bytes. It is not a signature. A signed record is only as trusted as the key set that the application keeps.
- The callable profile of the executor and the WASM bridge is still string and bool input and bool output. The edge executor line stays frozen until v1.0 (#162).
- The record format stays `entelos.record/1`. Keep the original lifecycle and engine builds with a store for exact replay. See [compatibility and retention](docs/SUPPORT.md#compatibility-and-retention).
- No new full security review of the Go path covers v0.5.0. Each PR had a review of its own diff. Codex had no usage for #194 and #195, so they had the local agy fallback review of [REVIEW.md](docs/REVIEW.md).

## v0.4.0 (2026-11-14)

This snapshot adds typed rule inputs, the approval API with two transports, a dashboard example, a store migration and a scope check. It also adds four proposed specs that no Host reads or writes yet.

### Reference code

- The engine implements the typed profile `cel-typed/v1` (#126). A rule declares a type for each input and for its output: `int`, `date`, `list` and `record`, and the two earlier types. The spec is [typed-profile-spec.md](src/cel-lifecycle/typed-profile-spec.md) (#141), the engine is in #144, and the vectors are in `src/cel-lifecycle/conformance/typed/`. `TestTypedPolicyFromProposalToReplay` runs one typed vector through the lifecycle Host, from the proposal to a replay of each receipt (#166). A refused input gives an answer and a receipt. The storage vector `typed-values.json` records the canonical bytes of each type. The executor and the WASM bridge refuse a typed bundle with `unsupported_profile`.
- The approval API 1.1 (#130) adds the reads `proposals` and `proposal`, the `reject` disposition of `decide`, a credentials file for each reviewer, a Host clock and the review index `state.reviews`. The spec is [APPROVAL.md](src/protocol/approval/APPROVAL.md) (#142), and the Go Host and the sidecar implement it (#149). `TestApprovalVectors` runs the vectors.
- The sidecar serves the approval API over HTTP (#156, #167). `POST /v1/request` accepts a signed claim (Ed25519) or, in local mode, a static token. The socket stays. A verdict from HTTP is the same record as a verdict from the socket. The binding has no TLS and no CORS headers.
- The Go module paths are now `github.com/mohanraj00/entelos/src/lifecycle-core` and `github.com/mohanraj00/entelos/src/cel-handoff` (#147, #151). A caller that imported the `example.org` paths must change its imports.
- A test checks the retry access for each write operation (#134, #143).

### Contents

- The approval dashboard (#157) moves to the mugham UI kit `v0.2.1`
  (commit `33678f8`), with contract format 2. Each card and the reason form
  show their author in the authorship slot, with a bound actor: the producer,
  the source attester, the runner, the reviewer, the policy or the page. Each
  proposal and each verdict is a separate card, because a card grid gives one
  author to all of its cards. Accept and Reject show a confirm step, and the
  receipt shows the verdict reference. The browser test checks the confirm
  step and the authors. Unit cases show that the kit refuses a fill with an
  `actions` key and a fill with a false author.

- The edge executor line is frozen until v1.0 (#162). The line is the WASM build,
  the browser bridge, the IndexedDB log, `ingest` of executor logs and the
  planned gomobile package. The code and tests stay, and CI still runs them.
  No new work starts on this line. A security fix is allowed. The change is in
  docs only: BACKLOG, SUPPORT, STABILITY, README and decisions 0001 and 0002.

- The `record` and `inspect` reads check the scope (#160). `record` refuses
  a record of another scope, or a record without a scope, with the new code
  `scope_mismatch`. `inspect` refuses with `scope_mismatch` if the store at
  the cursor holds such a record. The refusal has no record content. An
  unknown reference stays `artifact_unavailable`, so a caller that has a
  reference can still learn that it is in the store. The rule applies on
  each binding. This changes results of protocol 1.0 and 1.1. The owner
  accepted it as a security fix. The scope scan of `inspect` keeps one object
  in memory at a time, and gives `store_error` for an object over 16 MiB or
  for more than 256 MiB in total. The new vector file `approval-scope.json`
  has 11 steps.

- The new command `entelos-migrate` builds the review index of approval API
  1.1 for a store from v0.3.0 (#159). It adds one commit with
  `state.reviews` and changes no record. `--dry-run` writes nothing, and a
  second run writes nothing. It refuses an index that does not agree with
  the history. The Go package `lifecycle` adds the experimental
  `MigrateReviewIndex`.

  Upgrade steps for a v0.3.0 store:

  1. Stop each Host on the store.
  2. Make a copy of the bare repository.
  3. Run `entelos-migrate --repo <store.git> --dry-run` and examine the index.
  4. Run `entelos-migrate --repo <store.git>`. The action must be `migrated`.
  5. Start the Host. A v0.3.0 report names the v0.3.0 lifecycle build, so
     qualify an open proposal again before an accept.

  See [APPROVAL.md](src/protocol/approval/APPROVAL.md#migration-from-v030).

- [STABILITY.md](docs/STABILITY.md) names the stable surfaces (#147): the
  exported Go API, the wire protocol 1.0 and 1.1 and the record format
  `entelos.record/1`. It gives the promise for 0.x releases and the
  deprecation path. `make check` runs the pin in `tools/stability/`.

- [POLICY.md](src/protocol/policy/POLICY.md) proposes the policy verdict
  `entelos.policy/1` (#154): accept, refuse or route to a person, with
  fail-closed routes, a `suspend` stop control, an audit draw and a
  replayable receipt. A policy verdict never satisfies a human verdict
  requirement. `make check` runs its spec checker. No Host writes it yet.

- [examples/approval-dashboard](examples/approval-dashboard/README.md) adds an
  optional approval dashboard (#157) on the HTTP approval API with a local
  static token. It uses the mugham UI kit, pinned in `mugham.json`. The first entry
  of this section gives the current pin `v0.2.1`. The new sidecar flag `--http-static` serves its public files
  from the origin of the API. `make verify` runs its browser test: the list,
  the evidence view, one verdict and one API refusal.
  The Go package `protocol` adds the experimental `StaticHandler` and
  `NewStaticHandler`.

- [EVIDENCE.md](src/protocol/evidence/EVIDENCE.md) proposes the evidence format
  `entelos.evidence/1` (#148). It has a schema, 97 hand-written vectors and 14
  refusal rules. A check against groundgate 0.14.0 found fields that the first
  draft lost, and the revision adds them. No Host reads the format.

- [CONTENT.md](src/protocol/content/CONTENT.md) proposes the content proposal
  `entelos.content/1` (#153). A candidate has a kind, and the kind `content`
  is a manifest of digests with no CEL file. The spec has two schemas and
  hand-written vectors. No Host reads it. The Host support is v0.5 work.

### Upgrade steps

1. Change a Go import of `example.org/lifecycle-core` or `example.org/cel-handoff` to the new module path.
2. Migrate a store from v0.3.0 with `entelos-migrate`, as the steps above show. A v0.3.0 store without the review index gives `index_unavailable` for `proposals`, `proposal`, `qualify`, `decide` and `activate`.
3. Handle `scope_mismatch` from `record` and `inspect` in each client.
4. Run one store for each scope. The scope check refuses a mixed store, and it does not make a mixed store safe.

### Compatibility and limits

- From v0.4.0, [STABILITY.md](docs/STABILITY.md) promises these stable surfaces: the exported Go API that the pin lists, wire protocol 1.0 and 1.1, and the record format `entelos.record/1`. A 0.x release does not remove or change a stable declaration, and it does not change the schemas or vectors of an existing protocol version. It can add declarations and protocol minor versions. A deprecation has a notice period. The promise covers only what `tools/stability/check_stability.py` checks. A surface that the pin does not check is not stable. The APIs of the specs above and the edge executor line are not stable.
- The scope refusal changes results of protocol 1.0 and 1.1. The owner accepted it as a security fix.
- The specs for evidence, content and policy verdict are proposals. The vectors are hand-written, and a checker applies the rules to them. No Host reads or writes these formats.
- The callable profile of the executor and the WASM bridge is still string and bool input and bool output. The typed profile runs in the engine and the Go Host only.
- The edge executor line stays frozen until v1.0 (#162).
- The approval dashboard has no policy verdict to show from a Host. Only unit cases cover the policy verdict card.
- The sidecar HTTP binding is not a public internet boundary. Use a TLS front proxy.
- The record format stays `entelos.record/1`. Keep the original lifecycle and engine builds with a store for exact replay. See [compatibility and retention](docs/SUPPORT.md#compatibility-and-retention).
- No new full security review of the Go path covers v0.4.0. The v0.3.0 review of #136 covers v0.3.0 only. The owner accepted #160 as a security fix.

## v0.3.0 (2026-11-11)

- The Host rejects wrong field types, and both stores refuse state that is too deep to read (#136).
- Retrying a write requires current access to the affected functions (#133).
  An authorized retry still returns its original result and cursor.
- The standalone verifier supports spaces in the path to the WASM launcher.
- The sidecar requires a token on each request (#131). Each request frame
  starts with the prelude `ENT1` and the 64 hex digits of the token. The
  operator configures and rotates the token with `--token-file`. A missing or
  wrong token gives `unauthenticated` with the stage `authentication`. See
  [PROTOCOL.md](src/protocol/PROTOCOL.md#unix-socket-authentication).
- Go callers create the socket server with `protocol.NewServer(adapter,
  tokenFile)`. This replaces `protocol.Server{Adapter: ...}`.
- [ADD_CONSUMER.md](docs/ADD_CONSUMER.md) tells how to add a consumer example
  (#132). The [ci-trigger README](examples/ci-trigger/README.md) describes the
  PR CI example (#111).

### Compatibility and limits

- Record format remains `entelos.record/1`. The v0.1.0 store format is
  unsupported. Keep the original lifecycle and engine builds with the store
  for exact replay. See [compatibility and retention](docs/SUPPORT.md#compatibility-and-retention).
- This is a breaking change for sidecar clients. A v0.2.0 client must add
  the token prelude before it calls the v0.3.0 sidecar. The JSON messages
  remain at version 1.0.
- Use trusted local directories and a separate store for each scope. The
  callable profile accepts string and bool inputs and returns a bool.
- A local Codex security review covered the full Go approval and record path
  at `9b8261d` (#136). It found one P2 issue, and #137 fixed it. The review
  read the source. Its sandbox stopped `go test` and `go vet`, so the fix PR
  ran them.

## v0.2.0 (2026-11-06)

This snapshot replaces the Python lifecycle with a Go core and adds a local sidecar and a browser executor. It does not read stores from v0.1.0.

### Reference code

- The Go lifecycle core in `src/lifecycle-core` is now the only lifecycle Host (#95). The Python Host, its stores, its JSONL CLI and the protocol reference adapter are removed.
- `src/cel-lifecycle` keeps the specs, the storage port vectors and the scenario fixture.
- `scenario_test.go` runs the lifecycle scenario in the Go tests. Its expected values come from the fixture.
- `src/lifecycle-core/client/` holds the Python callers of the Go JSONL host: the scenario driver and the generic caller. The examples use them.
- `src/protocol/test_protocol.py` runs the session vectors only through the Go sidecar socket.
- `verify.py` has no parity stage. The `go-lifecycle-core` check runs the Go scenario, the storage port vectors and the Host tests.
- Each stored record has the field `format: "entelos.record/1"` (#78). A read of a record without this field, or with an unknown major version, fails closed with `unsupported_record_format`. The new `record-format` storage vector tests this rule. Stores from v0.1.0 are not migrated. See [SUPPORT.md](docs/SUPPORT.md).
- An expired or revoked activation fails closed by default (#82). The new diagnostics are `activation_expired` and `activation_revoked`. A proposal can declare `stale_mode` `continue_stale`. Then the loaded activation gives the answer, and the receipt has `stale` `true`. Each receipt records `stale_mode`, `stale` and `stale_reason`. See [CONTRACTS.md](docs/CONTRACTS.md#expiry-and-revocation).
- Each receipt records its provenance (#81): `executor_id`, `log_cursor`, `inputs_ref` and `selection.snapshot_ref`. The new `ingest` operation records an executor log, idempotent on the executor ID and the cursor. It keeps each answer and records the Host stale view in `host_stale_reason`. See [CONTRACTS.md](docs/CONTRACTS.md#receipt-provenance-and-ingest).
- At ingest, the Host compares the response `implementation_ref` with the snapshot `engine_build` (#108). A different or missing value gives `artifact_mismatch` and stops the ingest. This also applies to an entry from before this change.
- A storage port has conformance vectors and an in-memory adapter (#79). Both stores pass the storage vectors.
- A qualification runner interface separates the runner from the Host (#80).
- Wire protocol 1.0 is the versioned protocol for a client (#83). See [PROTOCOL.md](src/protocol/PROTOCOL.md).
- `entelos-sidecar` serves this protocol on a local Unix socket (#84). The new example is [sidecar-client](examples/sidecar-client/README.md).
- A WebAssembly build of the executor runs in a browser and keeps a receipt log (#93). The new example is [browser-executor](examples/browser-executor/README.md).

### Docs

- [Decision 0001](docs/decisions/0001-technology.md) records the technology choice (#85). [Decision 0002](docs/decisions/0002-browser-executor.md) records the browser executor design.
- The README, [SUPPORT.md](docs/SUPPORT.md) and [SECURITY.md](SECURITY.md) state the limits of the sidecar, ingest and the browser executor.

### Limits

- This snapshot does not read a v0.1.0 store, and there is no migration tool. See [SUPPORT.md](docs/SUPPORT.md).
- APIs, the protocol and the record format are unstable. The only callable profile is string/bool input and bool output.
- The sidecar has no authentication. The only access control is the file mode of the socket and of its directory. v0.3.0 adds a token (#131).
- An ingested receipt is the statement of the executor. The Host does not run the rule again, and a content reference is not a signature.
- The M5 security review covered the v0.1.0 Python path. Security-relevant M6 pull requests had a security review one at a time. No one repeated the review for the full Go path as one review.
- The `inspect` and `record` reads do not check the scope. Use one store for each scope.
- A `request_key` with a NUL byte gives `store_error` on the Git store. The in-memory store accepts this key.
- Linux and macOS are tested. Windows is not tested. `verify.py` needs a Chromium browser.

## v0.1.0 (2026-11-04)

The first snapshot. It contains the runnable reference and its docs.

### Reference code

- `src/cel-handoff`: a Go host that prepares and evaluates typed CEL rules. Rules accept named string and bool inputs and give a bool result.
- `src/cel-lifecycle`: a Python lifecycle that records each proposal, qualification, approval, dated activation, answer, replay and restatement in a local Git store.
- Three consumer examples that reuse `src/` without copies: `record-filter`, `ci-trigger` and `tier-eligibility`.
- `verify.py`: the full check runner. CI runs it on `ubuntu-24.04` and `macos-15`.

### Docs

- The README states the problem first, gives one demo command and puts the limits at the top.
- [WALKTHROUGH.md](docs/WALKTHROUGH.md), [CORRECTION.md](docs/CORRECTION.md) and the [decision path diagram](docs/diagrams/decision-path.svg).
- [COMPARISON.md](docs/COMPARISON.md) compares OPA/Rego, Cedar, GoRules ZEN, feature-flag services and ordinary code plus Git.
- [CONTRACTS.md](docs/CONTRACTS.md) gives the callable cases and the author fixture fields.
- `CONTRIBUTING.md`, issue templates and a review path for a contributor without an agent harness.

### Security fixes

The M5 security review of the approval and record path found five issues. #74 fixed them.

- An answer now requires the lifecycle build that the qualification report names. Before, only the report check compared the build.
- The reviewer who approves must be different from the producer and the policy runner.
- A replay receipt records `matches_original`, which shows if the new answer is the same as the recorded answer.
- A wrong JSON type in `inputs_json` and very deep JSON nesting now give a fault. Before, they stopped the CLI stream.

### Limits

- The record format has no version field. A later snapshot can change the records, and new code may not load an old store. See [SUPPORT.md](docs/SUPPORT.md).
- APIs are unstable. The only callable profile is string/bool input and bool output.
- Linux and macOS are tested. Windows is not tested.
- The `inspect` and `record` reads do not check the scope. Use one store for each scope.
- A `request_key` with a NUL byte gives `store_error`. The state does not change.
