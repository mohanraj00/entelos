# Implemented contract map

This map points to executable reference contracts. It is not a separately versioned standard.

| Surface | Source | Behavior |
|---|---|---|
| Typed host | [handoff.go](../cel-handoff/handoff.go) | Exact bundle/build, declared caller fields, disjoint fixed values, string/bool inputs and bool result. |
| Authoring | [prepare.py](../cel-handoff/tools/prepare.py) and pinned upstream CEL CLI | One environment describes the author/host declarations; tests remain host-owned. |
| Calling | [client.py](../cel-lifecycle/client.py) | Discover a typed contract, then send named values and expected bundle identity without source or tests. |
| Lifecycle | [lifecycle.py](../cel-lifecycle/lifecycle.py) | Qualification/report binding, separate admission, interval selection, preparation, receipt and correction operations. |
| Local storage | [git_store.py](../cel-lifecycle/git_store.py) | Immutable objects, captured historical roots, request-key recovery and single-root compare-and-swap. |
| Application boundary | [adapter.py](../ci-trigger/adapter.py) | Project a nested event, retain raw/projection audit links; no atomic webhook ingress or pipeline effect is implemented. |

The JSON CLI schemas and typed faults are defined in source. Qualification success does not activate a proposal. Caller discovery must be checked again after source/profile/bundle changes. Invalid input and a successful false result remain different outcomes.

Replay retains the original input and execution closure. Restatement names a later accepted root and computes a separate answer for the retained original input. A correction to application facts therefore requires a new input/call and an explicit relationship supplied by the application; restatement does not reread a live fact source.

Applications supply source authority, independent expectations, admission/learning policy, authentication, retention and side effects. See [INTEGRATION.md](../INTEGRATION.md), [SECURITY.md](../SECURITY.md) and [SUPPORT.md](../SUPPORT.md) before treating the local reference as an application component.
