# Activation record

This file specifies a generic activation record. Issue [#105](https://github.com/mohanraj00/entelos/issues/105) tracks it. An activation record is one self-contained statement of an activation: the candidate, its base, the receipts, the actors, the partition, the interval and the applicability values. A consumer, for example a store with gated collections, verifies it offline and makes no call to entelos.

This file also specifies the suspension record: one self-contained statement of a suspension. Issue [#211](https://github.com/mohanraj00/entelos/issues/211) tracks it. See [Suspension record](#suspension-record).

Status: **implemented, experimental**. The Go Host writes and signs the two records, and calls the optional publisher ([activation.go](../../lifecycle-core/activation.go)). The schemas and the vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors, without a Host. The Go tests `TestActivationVectors` and `TestSuspensionVectors` apply the Go mapping and the Go rules to the same vectors.

## Terms

| Term | Meaning |
|---|---|
| Event | The stored `enable` event of an activation. [SIGNED.md](../signed/SIGNED.md) calls it an activation record of the kind `event`. |
| Activation record | A stored record of the kind `activation`. The Host writes it from the event and the records that the event names. |
| Signed activation record | A signed record ([SIGNED.md](../signed/SIGNED.md)) of an activation record. |
| Consumer | A system that receives a signed activation record and a key set, and checks them with no call to entelos. |
| Suspension record | A stored record of the kind `suspension`. The Host writes it from the `suspend` event. |
| Published record | An activation record or a suspension record. |
| Publisher | An optional adapter of the embedding host. It gives a signed published record to a consumer. |

## Decision

1. The activation record is a new record kind, `activation`. It is a lifecycle record of the format `entelos.record/1`, as each other record of the Host.
2. The Host writes one activation record for each `enable` event, in the same transaction, when the Host has a signing key. It signs the activation record as it signs the event. The state field `activations` maps the event reference to the activation record reference. Without a signing key, the Host writes no activation record and no `activations` field, and its behavior does not change.
3. The record copies its values from the event, the snapshot, the qualification report and the decision. It adds no value that the store does not have. Thus the record is a function of the stored records and the request key.
4. The record has no time field. A retry of the same `activate` request makes the same bytes, the same reference and the same signature.
5. The publisher is optional. Entelos core does not need a consumer or a store. The Host calls the publisher before it commits the activation. If the publisher refuses, the activation does not become active, and the Host stores no record.
6. The event and its signed record do not change. The activation record is an additional record.
7. The Host writes one suspension record for each `suspend` event. Decisions 2 to 6 apply to it: the same transaction, only with a signing key, no time field, and a call to the publisher before the commit. The suspension record is a different record kind, `suspension`. The signature covers the record kind ([SIGNED.md](../signed/SIGNED.md)), so a consumer cannot read a suspension record as an activation record.

The reason for decision 5: a consumer can refuse an update, for example because its head is not the `base`. Then the update must not become active in entelos either. A commit can still fail after the consumer accepts. Decision 4 makes the retry safe, because the consumer gets the same bytes again.

## Activation record

An activation record is one JSON object. [`schema/activation.schema.json`](schema/activation.schema.json) gives its fields. A field that is not in this table is not permitted.

| Field | Type | Value |
|---|---|---|
| `format` | string | `entelos.record/1`. |
| `scope` | string | The `scope` of the event. |
| `function` | string | The `function` of the event. For a content candidate, it is the subject `content:<collection>/<partition>`. |
| `event_ref` | string | The reference of the event. |
| `request_key` | string | The `request_key` of the `activate` request, or an empty string if the request has none. It is for audit and idempotence. |
| `candidate` | object | `{"kind", "ref"}`. For a content snapshot, the `candidate` of the snapshot. For a CEL snapshot, `{"kind":"cel-bundle","ref":<bundle_ref>}`. |
| `base` | string | The candidate `ref` that this candidate changes, or an empty string. For a content snapshot, the `base` of the snapshot. For a CEL snapshot, the `bundle_ref` of its `base_snapshot`, or an empty string if it has none. |
| `collection` | string | Only for a content candidate: the `collection` of the snapshot. |
| `partition` | string | Only for a content candidate: the `partition` of the snapshot. |
| `from` | string | The `from` date of the event. The interval includes this date. |
| `until` | string | The `until` date of the event. The interval does not include this date. |
| `source_ref` | string | The `source_ref` of the snapshot. |
| `snapshot_ref` | string | The `snapshot_ref` of the event. |
| `report_ref` | string | The `report_ref` of the event: the qualification receipt. |
| `decision_ref` | string | The `decision_ref` of the event: the approval receipt. |
| `producer` | string | The `producer` of the snapshot. |
| `runner` | string | The `runner` of the report. |
| `approver` | string | The `actor` of the decision. |
| `activator` | string | The `actor` of the event. |
| `targets` | array | The `targets` of the event: the references of the updates that the event masks. |
| `supersedes` | array | The `supersedes` of the event, or an empty array. |
| `applicability` | object | The `applicability` of the event ([#106](https://github.com/mohanraj00/entelos/issues/106)), or an empty object. It has the values of each item, by `item_id`. |

A reference is `sha256:` and 64 lowercase hex digits. A date has the form `YYYY-MM-DD`. The record does not copy the text of the manifest, of the source or of the evidence. The consumer gets them by reference.

## Mapping

This table gives the origin of each field. The Host reads each record in the same transaction as the event.

| Origin | Fields |
|---|---|
| The event | `scope`, `function`, `from`, `until`, `snapshot_ref`, `report_ref`, `decision_ref`, `targets`, `supersedes`, `applicability`; `actor` as `activator`; its reference as `event_ref` |
| The snapshot | `candidate` (or `bundle_ref` for a CEL snapshot), `base` (or the `base_snapshot` for a CEL snapshot), `collection`, `partition`, `source_ref`, `producer` |
| The base snapshot of a CEL snapshot | its `bundle_ref` as `base` |
| The report | `runner` |
| The decision | `actor` as `approver` |
| The request | `request_key` |

The Host writes an activation record only for an `enable` event. A `suspend` event has no activation record. It has a [suspension record](#suspension-record).

To read a record, use the `inspect` operation, find the event reference in `activations`, and then find the activation record reference in `signatures`. The `record` operation with the kind `signed` gives the signed record.

## Rules

A consumer applies these rules to the record bytes, after the checks of [SIGNED.md](../signed/SIGNED.md), in this order. The first rule that fails gives the result. The Go function `VerifyActivation` does the checks of SIGNED.md and then these rules. Its refusal has the stage `activation`, and the message starts with the rule name.

| Step | Rule | Result |
|---|---|---|
| 1 | The `record_kind` of the signed record is `activation`. | `unexpected_record_kind` |
| 2 | The record bytes are one JSON object. No object in the text has the same member name two times. | `invalid_activation`, rule `json` |
| 3 | `format` is `entelos.record/1`. | `unsupported_record_format` |
| 4 | Each member name in the record is a field of the record table. | `invalid_activation`, rule `unknown_field` |
| 5 | The record agrees with the schema. | `invalid_activation`, rule `schema` |
| 6 | The record has `collection` and `partition` if, and only if, the `kind` of `candidate` is `content`. | `invalid_activation`, rule `candidate_fields` |
| 7 | For a content candidate, `function` is `content:` and `collection`, `/` and `partition`. | `invalid_activation`, rule `subject` |
| 8 | Each date is a real calendar date, and `from` is before `until`. | `invalid_activation`, rule `interval` |

The refusal of rule `unknown_field` gives the path of the first unknown member name, in the order of the text, for example `candidate.digest`. The names in `applicability` are data, not fields.

A record that passes all of the rules is valid. The result gives the record. A valid record shows what the signer activated. These checks stay with the consumer:

- The `scope`, the `function`, or the `collection` and the `partition`, are the head that the consumer keeps.
- The `base` is the candidate that the consumer has as the current head. Else the consumer refuses the update.
- The `candidate` names the bytes that the consumer receives. For content, the consumer calculates the SHA-256 of the manifest text and compares it with the `ref`.
- The interval and the `applicability` values are the filters that the consumer applies. Entelos does not interpret them.
- The key set is the one that the consumer trusts for this signer.

## Suspension record

A suspension record is one JSON object. [`schema/suspension.schema.json`](schema/suspension.schema.json) gives its fields. A field that is not in this table is not permitted.

| Field | Type | Value |
|---|---|---|
| `format` | string | `entelos.record/1`. |
| `scope` | string | The `scope` of the event. |
| `function` | string | The `function` of the event: the subject of the suspension. For a content subject, it is `content:<collection>/<partition>`. |
| `event_ref` | string | The reference of the `suspend` event. |
| `request_key` | string | The `request_key` of the `suspend` request, or an empty string if the request has none. It is for audit and idempotence. |
| `from` | string | The `from` date of the event. The interval includes this date. |
| `until` | string | The `until` date of the event. The interval does not include this date. |
| `targets` | array | The `targets` of the event: the references of the events that the suspension masks. It has 1 reference or more. |
| `actor` | string | The `actor` of the event: the actor that suspended. |

The record copies each value from the `suspend` event and the request key. Thus the record is a function of the stored event and the request key. A retry of the same `suspend` request makes the same bytes, the same reference and the same signature. The record does not copy the `reason` of the event. The reason stays in the event, and a consumer gets it by the `event_ref`.

A target is the reference of an event. For an activation, it is the `event_ref` of an activation record that the consumer has. A suspension masks its targets only on the dates of its interval. A later activation can name the suspension event in its `targets`, and then it masks the suspension.

The Host writes a suspension record only for a `suspend` event. An `enable` event has no suspension record. The state field `suspensions` maps the event reference to the suspension record reference. Without a signing key, the Host writes no suspension record and no `suspensions` field.

To read a record, use the `inspect` operation, find the event reference in `suspensions`, and then find the suspension record reference in `signatures`. The `record` operation with the kind `signed` gives the signed record.

### Suspension rules

A consumer applies these rules to the record bytes, after the checks of [SIGNED.md](../signed/SIGNED.md), in this order. The first rule that fails gives the result. The Go function `VerifySuspension` does the checks of SIGNED.md and then these rules. Its refusal has the stage `suspension`, and the message starts with the rule name.

| Step | Rule | Result |
|---|---|---|
| 1 | The `record_kind` of the signed record is `suspension`. | `unexpected_record_kind` |
| 2 | The record bytes are one JSON object. No object in the text has the same member name two times. | `invalid_suspension`, rule `json` |
| 3 | `format` is `entelos.record/1`. | `unsupported_record_format` |
| 4 | Each member name in the record is a field of the record table. | `invalid_suspension`, rule `unknown_field` |
| 5 | The record agrees with the schema. | `invalid_suspension`, rule `schema` |
| 6 | Each date is a real calendar date, and `from` is before `until`. | `invalid_suspension`, rule `interval` |

The refusal of rule `unknown_field` gives the first unknown member name, in the order of the text.

A record that passes all of the rules is valid. The result gives the record. A valid record shows what the signer suspended. These checks stay with the consumer:

- The `scope` and the `function` are the head that the consumer keeps.
- Each target is an event that the consumer knows. A target that the consumer does not know can be an activation that it did not receive, or a suspension.
- The interval gives the dates on which the consumer stops the targets. Entelos does not apply the suspension for the consumer.
- The key set is the one that the consumer trusts for this signer.

A suspension record and an activation record never pass the rules of the other kind:

- Step 1 of each rule table refuses the other `record_kind`.
- The signature covers the media type of the record kind. Thus a signed record with a changed `record_kind` fails the signature check of SIGNED.md.
- The record tables are different. A suspension record has the field `actor` and no `candidate`. Each one fails rule `unknown_field` or rule `schema` of the other kind.

## Publisher

The publisher is an adapter of the embedding host. The Go interface is `Publisher`, and `Host.SetPublisher` installs it. It has one operation, `Publish`: give the bytes of the published record and of its signed record to a consumer, and get an accept or a refusal with a reason. The bytes are the bytes that the store keeps. The `record_kind` of the signed record tells the consumer if the record is an activation record or a suspension record.

The rules below are the same for the `activate` write with its activation record and for the `suspend` write with its suspension record.

- The Host calls the publisher in the write, after it stages and signs the records, and before it commits them. The store calls the publisher only after it owns the root update: the in-memory store holds its lock, and the Git store holds the lock of the root ref in one `git update-ref --stdin` transaction. Thus a write that loses the compare-and-swap to another write does not publish.
- The publisher must not call the Host or the store, because the store holds its lock during the call.
- The call has a time limit. The default limit is 10 seconds, and `Host.SetPublishTimeout` sets another limit. The publisher gets a Go context with this deadline, and it must stop its work when the context is done.
- At the limit, the Host commits nothing and releases the store lock. The result is `publish_timeout` with the stage `publish`. The Host does not wait for the publisher. Thus the limit is also the longest time that one publisher call stops the other writes.
- After `publish_timeout`, the caller does not know if the consumer has the record. A publisher that ignores the context can give the record to the consumer after the limit. This is the same case as a commit that fails after an accept, and the same rule applies: a retry gives the same bytes.
- On an accept, the Host commits the write.
- On a refusal, the Host commits nothing. The result is `publish_refused` with the stage `publish`, and the message has the reason of the consumer. Thus a refused suspension does not suspend in entelos either.
- On a failure of the commit after an accept, the consumer has a record that the store does not have. A retry of the same request makes the same bytes, and the consumer gets them again. The consumer must accept a record that it already has.
- A retry of a stored request is a reuse. The Host does not call the publisher again.
- A Host with a publisher and no signing key refuses each `activate` and each `suspend` with `publish_unsigned` and the stage `publish`, because a consumer cannot verify the record. It stores nothing.
- A Host with a publisher refuses each `activate` and each `suspend` with `publish_unsupported` and the stage `publish` if its store is not one of the stores of the Go package. Only those stores call the publisher under the lock.
- A publisher that accepts only activation records must refuse a suspension record. It must not read the bytes as an activation.

| Code | Use |
|---|---|
| `publish_refused` | The publisher refused the activation record or the suspension record. The message has the reason. |
| `publish_timeout` | The publisher gave no answer in the time limit. The store committed nothing. |
| `publish_unsigned` | The Host has a publisher and no signing key. |
| `publish_unsupported` | The Host has a publisher and a store that does not call it under the lock. |

## Stale lock of the root ref

The Git store moves its root in one `git update-ref` command. That command makes the lock file `refs/lifecycle/state.lock` in the repository and removes it when it completes. During a publisher call, the lock file is there for the time of the call.

A test with git 2.54.0 on macOS shows these facts, for a `git update-ref --stdin` process after its `prepare` command:

- If the Host process stops, the `git` process gets the end of its input. It then removes the lock file and does not move the root.
- If the `git` process gets SIGTERM, it removes the lock file.
- If the `git` process gets SIGKILL, the lock file stays. For example, a container runtime can stop all processes of a container in this way.

A lock file that stays is a stale lock file. No process owns it. Each later write fails with `store_error`, and the root does not move. Reads continue to work. The message of `git` tells the reader to remove the file by hand. Do not do that before the check below: the Host adds the name of the recovery command to the message.

### How the store knows that no update is in progress

The repository has the file `lifecycle-root-update.flock`. It is empty, and the Git store makes it at the first write. Each update of the root ref takes a shared `flock` lock on this file. The `git` process of the update keeps the file open until it stops. The operating system releases the lock when the last process closes the file, also after SIGKILL.

The recovery takes an exclusive `flock` lock on the same file, and it does not wait. Thus the recovery gets the lock only if no Host is in an update and no `git` process of an update is alive. Then no process owns the lock file of the root ref, and the lock file is stale. While the recovery holds the exclusive lock, a new update waits for 1 second maximum and then fails with `store_error`.

The removal of a stale lock file does not change the root. An update moves the root only when `git` renames its lock file to the ref file. A lock file that stays is a lock file that `git` did not rename.

### Recovery procedure

Do these steps on the system that has the repository:

1. Run `entelos-unlock --repo <store.git> --check`. The command removes nothing.
2. Read the field `lock` of the result. If it is `absent`, no lock file is there, and the store needs no recovery. Stop.
3. If the result is the code `update_in_progress`, a write is in progress. Wait for the publish time limit and do step 1 again. If the code stays, stop the callers that send writes, because each failed write also holds the marker lock for a short time.
4. If `lock` is `stale`, run `entelos-unlock --repo <store.git>`. The field `lock` of the result is then `removed`.
5. Compare the field `root` of step 1 and of step 4. The two values are the same.
6. Send the write again with the same request key. If the stopped write did not commit, the consumer gets the same bytes again.

Step 4 does the check of step 1 again under the same exclusive lock, and removes the lock file only then. Thus it is also safe to do step 4 without step 1.

The command prints one JSON line. On success, the line has `"status":"ok"` and the fields `lock` and `root`, and the exit status is 0. On a refusal, the line has the code, and the exit status is 1. The Go function `GitStore.RecoverRootLock` does the same work for an embedding host.

| Code | Use |
|---|---|
| `update_in_progress` | An update of the root ref is in progress. The recovery removed nothing. The stage is `store`. |
| `invalid_store` | The lock path of the root ref is not a regular file, or the repository is not there. The recovery removed nothing. |

Do not remove the lock file by hand, unless the limits below make the command unusable. A removal by hand during an update breaks the lock of that update. Its compare-and-swap is then not safe.

The check has these limits:

- It sees only a Host of this version or later. A Host of an earlier version takes no lock on the marker file.
- It sees only the processes on the same operating system kernel. For a repository on a network file system, a Host on a different machine is not visible. Stop each Host of the store before you do the recovery.
- It needs a Unix system with `flock`. On each other system, the Git store takes no lock on the marker file, and the recovery gives `store_error`.
- On a file system that does not support `flock`, the Git store takes no lock on the marker file, and the recovery gives `store_error`.
- It sees only the updates that the Git store makes. A `git` command that a person runs on the repository is not visible. Run no other `git` command on the repository during the recovery.
- It examines only the lock file of the root ref. It does not repair other damage of the repository.
- The lock file path is the path of the Git files ref format. This is the default format of git 2.54.0, and the Git store makes its repository with the default format. A repository with the reftable format has no such file, and the recovery gives `absent`.

## Vectors

[`vectors/activation-cases.json`](vectors/activation-cases.json) has two lists.

Each item of `mappings` has:

- `name`: a unique name;
- `records`: the `event`, the `snapshot`, the `report`, the `decision`, and for a CEL snapshot with a base the `base_snapshot`, as objects;
- `event_ref` and `request_key`;
- `expect`: the activation record, or `{"none": true}` for an event that has no activation record.

Each item of `records` has:

- `name`: a unique name;
- `record_kind` and `record`: the record kind of the signed record and the record text, as a string;
- `expect`: `{"valid": true}`, or `{"valid": false, "code": <code>}` with `"rule"`, and `"field"` for rule `unknown_field`.

`check_spec.py` does these checks:

1. It applies the mapping of this file to each item of `mappings`, and compares the result with `expect`.
2. It checks each expected record of `mappings` against the rules.
3. It applies the rules of this file to each item of `records`, and compares the result with `expect`.
4. It checks that each rule and each code of the rules table has one or more cases.
5. A self-test changes a copy of the vectors and checks that the checker reports each change.

[`vectors/suspension-cases.json`](vectors/suspension-cases.json) has the same two lists for the suspension record. Each item of its `mappings` has only the `event` in `records`, and its `expect` is the suspension record, or `{"none": true}` for an event that has no suspension record. `check_spec.py` does the five checks on this file with the suspension mapping, the suspension schema and the suspension rules table.

The Go tests `TestActivationVectors` and `TestSuspensionVectors` do checks 1 to 3 with the Go mapping and the Go rules. Agreement of the checker, the Go code and the vectors shows that the three agree with each other. It does not show that the text is correct.

## Limits

- The wire protocol and the sidecar have no publisher setting. An embedding Go host calls `SetPublisher`.
- No consumer adapter is in this repository. The Go tests use a test publisher.
- The record has no trusted time. The signature shows who signed, not when.
- The Host stores no record of a refused publish. The caller gets the code and the reason.
- An activation before the signing key has no activation record, and a suspension before the signing key has no suspension record.
- A suspension record does not show that its targets exist or that they overlap its interval. The Host checks this at the `suspend` write. A consumer can compare each target with the events that it knows.
- A slow publisher holds the store lock for the publish time limit maximum, and the other writes wait or fail for that time. The default limit is 10 seconds.
- The Host does not stop a publisher that ignores its context. Such a call continues in the Host process after `publish_timeout`.
- If the `git` process stops with SIGKILL during a write, the Git store keeps the lock file of the root ref. The Host does not remove it automatically. A person runs `entelos-unlock`, as [Stale lock of the root ref](#stale-lock-of-the-root-ref) says, with the limits of that section.
