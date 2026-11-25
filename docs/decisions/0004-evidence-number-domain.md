# 0004 One number domain for evidence and the wire

- Status: proposed
- Date: 2026-11-25
- Decided by: the maintainer, when the maintainer merges the Host change
- Issue: [#208](https://github.com/mohanraj00/entelos/issues/208)
- Spec: [EVIDENCE.md](../../src/protocol/evidence/EVIDENCE.md#numbers)

## Question

How can one evidence document pass Host admission and travel through approval API 1.2 over the socket and HTTP?

## Decision

1. **Evidence uses the wire number domain.** Each number is an integer in the range ±(2^53 − 1), including numbers in attached objects.
2. **Judgments use parts per million and boxes use thousandths of a point.** A producer declares those units in its attached producer data. The marker distinguishes converted values from earlier integers in the same fields.
3. **The experimental format keeps the name `entelos.evidence/1`.** Admission refuses earlier numeric forms with `invalid_evidence`, rule `schema`. Earlier documents that already meet the number rules continue to pass admission.

## Reasons

| | Integers in evidence | Decimals in a new protocol minor |
|---|---|---|
| Client support | Works on protocol 1.2 | Both ends must use the new version |
| Work for producers | Convert two fields and encode attached numbers in range | Keep existing fractions |
| Canonical bytes | Use the existing wire number domain | Specify decimal precision, spelling and canonicalization |

Protocol 1.0 through 1.2 are stable, and the evidence format is experimental. Fixed-point integers preserve useful precision in both fields and let producers keep using 1.2.

## Consequences

- The 1.2 attachment shape and wire number checks stay in place. The Go Host applies the new number rules when it checks admission.
- Producers re-encode affected documents and compute a new content manifest evidence digest when they cite one.
- Stored snapshots keep their bytes. An accept or activation of earlier content can refuse its retained evidence under the new admission rules. A producer can propose a new snapshot with converted evidence.
