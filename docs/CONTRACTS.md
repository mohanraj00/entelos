# Implemented contract map

This map points to executable reference contracts. It is not a separately versioned standard.

| Surface | Source | Behavior |
|---|---|---|
| Typed host | [handoff.go](../src/cel-handoff/handoff.go) | Exact bundle/build, declared caller fields, disjoint fixed values, string/bool inputs and bool result. |
| Authoring | [prepare.py](../src/cel-handoff/tools/prepare.py) and pinned upstream CEL CLI | One environment describes the author/host declarations; tests remain host-owned. |
| Calling | [client.py](../src/cel-lifecycle/client.py) | Discover a typed contract, then send named values and expected bundle identity without source or tests. |
| Lifecycle | [lifecycle.py](../src/cel-lifecycle/lifecycle.py) | Qualification/report binding, separate admission, interval selection, preparation, receipt and correction operations. |
| Local storage | [git_store.py](../src/cel-lifecycle/git_store.py) | Immutable objects, captured historical roots, request-key recovery and single-root compare-and-swap. |
| Application boundary | [adapter.py](../examples/ci-trigger/adapter.py) | Project a nested event, retain raw/projection audit links; no atomic webhook ingress or pipeline effect is implemented. |

The JSON CLI schemas and typed faults are defined in source. Qualification success does not activate a proposal. Caller discovery must be checked again after source/profile/bundle changes. Invalid input and a successful false result remain different outcomes.

Replay retains the original input and execution closure. Restatement names a later accepted root and computes a separate answer for the retained original input. A correction to application facts therefore requires a new input/call and an explicit relationship supplied by the application; restatement does not reread a live fact source.

Applications supply source authority, independent expectations, admission/learning policy, authentication, retention and side effects. See [INTEGRATION.md](INTEGRATION.md), [SECURITY.md](../SECURITY.md) and [SUPPORT.md](SUPPORT.md) before treating the local reference as an application component.

## Lifecycle request records

The CLI accepts one object per call: `op` plus that operation's fields. The host installs the roles in the context file, so a request never names its own role. Every write operation also requires `expected_revision` and `request_key`; read operations reject them. An unknown field is `invalid_request`.

| Operation | Host role | Accepted fields |
|---|---|---|
| `source` | reviewer | `function`, `from`, `until`, `text`, `supersedes`, `reason` |
| `plan` | reviewer | `source_ref`, `cases`, `origin`, `supersedes` |
| `propose` | author | `function`, `from`, `until`, `base_snapshot`, `source_ref`, `bundle`, `reason` |
| `qualify` | runner | `snapshot_ref`, `plan_ref` |
| `decide` | reviewer | `report_ref`, `reason` |
| `activate` | reviewer | `decision_ref`, `targets` |
| `suspend` | reviewer | `function`, `from`, `until`, `targets`, `reason` |
| `load` | worker | `function`, `date` |
| `invoke` | caller | `function`, `date`, `expected_bundle_ref`, `inputs` |
| `replay` | reader | `receipt_ref`, `inputs_json`, `reason` |
| `restate` | reader | `receipt_ref`, `cursor`, `reason` |
| `observe` | observer | `receipt_ref`, `route`, `message` |
| `claim` | maintainer | `task_id`, `checkpoint` |
| `checkpoint` | maintainer | `task_id`, `checkpoint`, `token`, `status`, `note`, `correction_snapshot` |
| `takeover` | reviewer | `task_id`, `checkpoint`, `owner`, `reason` |
| `ack` | worker | `event_ref` |
| `describe` | caller | `function`, `date` |
| `resolve` | reader | `function`, `date`, `cursor` |
| `inspect` | reviewer | `cursor` |
| `record` | reviewer | `ref`, `kind`, `cursor` |
| `receipt` | reader | `receipt_ref` |

`source.supersedes` and `plan.supersedes` default to an empty list. `activate.targets` defaults to an empty list. `checkpoint.correction_snapshot` defaults to an empty string. `describe`, `resolve`, `inspect`, `record` and `receipt` are read operations; the other operations are writes.

## Proposal base rule

`propose` targets one function, scope and date interval. `base_snapshot` must equal the current proposal head for that function and scope. The head advances on every stored proposal, including a proposal that did not pass qualification or was never activated, so a different value is `stale_proposal`. The first proposal for the key uses an empty `base_snapshot`.

The bundle must name all four components. The manifest's `function_id` must equal the proposal's `function`, or the call is `contract_changed`. When `base_snapshot` is not empty, the new manifest must keep the base manifest's `environment_ref`, `caller_variables`, `profile` and `limits`; a difference is `contract_changed`. A later proposal may change the rule and the fixed data.

## Content references for a stage bundle

A content reference is `sha256:` plus the lowercase hex SHA-256 of `media type + NUL + exact bytes`. Lifecycle records use `application/vnd.cel-lifecycle.<kind>+json`. The four bundle components use:

| Component | Media type |
|---|---|
| `manifest.json` | `application/vnd.cel-handoff.bundle+json` |
| `env.json` | `application/json` |
| `rule.cel` | `text/plain` |
| `fixed.json` | `application/json` |

To change a fixed value for a stage, replace the fixed bytes, compute the new reference with the `application/json` media type, write it into the manifest's `fixed_ref`, encode the manifest as canonical JSON (sorted keys, UTF-8, no insignificant whitespace, trailing newline), and compute the manifest reference with the bundle manifest media type. The manifest's `environment_ref` and `source_ref` follow the same rule when their bytes change. The runtime reports the bundle identity in `describe`; the lifecycle records it as the snapshot's `bundle_ref` and `engine_build`.
