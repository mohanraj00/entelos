# Signed records

This file specifies the signature on an approval record and on an activation record, and the offline check of that signature. Issue [#155](https://github.com/mohanraj00/entelos/issues/155) tracks it.

Status: **implemented in the Go core, experimental**. The Go verifier is [`sign.go`](../../lifecycle-core/sign.go). The vectors in [`vectors/signed-cases.json`](vectors/signed-cases.json) have expected values that a person wrote from this text. No verifier output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors with its own Ed25519 code. `TestSignedVectors` runs the same vectors on the Go verifier.

## Terms

| Term | Meaning |
|---|---|
| Record | One stored lifecycle record. Its bytes are the canonical record bytes that the Host wrote. |
| Approval record | A record of the kind `decision`. |
| Activation record | A record of the kind `event`. An `enable` event and a `suspend` event are both activation records. |
| Signed record | A record of the kind `signed`. It holds the bytes of one approval or activation record, and a signature on them. |
| Signer | The Host that holds a private signing key. |
| Key set | A public document with the public keys of one signer and the status of each key. |
| Consumer | A system that receives a signed record and a key set, and checks them with no call to entelos. |

## Signature

The algorithm is Ed25519 ([RFC 8032](https://www.rfc-editor.org/rfc/rfc8032), section 5.1). Its name in each document is `ed25519`.

The signature covers this byte string:

```text
"entelos.signature/1" 0x00 <media type> 0x00 <record bytes>
```

- `entelos.signature/1` is the domain of the signature, in ASCII. A change to this layout gets a new number.
- The media type is `application/vnd.cel-lifecycle.<record_kind>+json`, in ASCII. Thus the signature also covers the record kind.
- The record bytes are the exact bytes of the stored record. A verifier never makes the bytes again from a JSON value.

The media type, the zero byte and the record bytes are the stored envelope of the record. The record reference is `sha256:` and the SHA-256 value of this envelope in lowercase hex. A record reference is an identity. It is not a signature.

The key identifier is `ed25519:` and the SHA-256 value of the 32 bytes of the public key in lowercase hex. A key identifier is also an identity. The signature does not cover the key identifier. A signed record that names a different key fails the signature check.

Each binary value is base64 text with the standard alphabet and padding ([RFC 4648](https://www.rfc-editor.org/rfc/rfc4648), section 4). The text has no white space, and it is the text that an encoder gives for the bytes.

## Signed record

A signed record is one JSON object. [`schema/signed.schema.json`](schema/signed.schema.json) gives its fields. A field that is not in this table is not permitted.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.record/1`. The record format rule applies. |
| `scope` | string | The scope of the record. 1 character or more. |
| `function` | string | The function of the record. 1 character or more. |
| `record_kind` | string | `decision`, `event` or `activation`. The kind `activation` is the self-contained record of [ACTIVATION.md](../activation/ACTIVATION.md). |
| `record_ref` | string | The record reference of the record bytes. |
| `record` | string | The record bytes in base64. The text has 699,052 characters maximum, which is the text of 512 KiB. |
| `algorithm` | string | `ed25519`. |
| `key_id` | string | The key identifier of the key that signed the record. |
| `signature` | string | The 64 bytes of the signature in base64. |

`scope` and `function` are copies of the fields of the record, with no length limit of their own. The limit of `record` also limits them, because a valid signed record has the same values in its record bytes. The Host uses `scope` to give a signed record only in its own scope. The signature covers the fields in the record bytes, and the verifier compares the copies with them.

## Key set

A key set is one JSON object. [`schema/keyset.schema.json`](schema/keyset.schema.json) gives its fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.keyset/1`. |
| `signer` | string | A name for the signer. 1 to 256 characters. |
| `keys` | array | 1 to 64 keys. |
| `keys[].key_id` | string | The key identifier of `public_key`. No two keys have the same identifier. |
| `keys[].algorithm` | string | `ed25519`. |
| `keys[].public_key` | string | The 32 bytes of the public key in base64. |
| `keys[].status` | string | `active`, `retired` or `revoked`. |
| `keys[].note` | string | Optional. 1 to 256 characters. |

The status of a key has this meaning:

| Status | The signer | The verifier |
|---|---|---|
| `active` | Signs new records with this key. | Accepts the key. |
| `retired` | Signs no new record with this key. | Accepts the key. The result gives the status. |
| `revoked` | Signs no new record with this key. | Refuses each record of this key. |

The owner of the signer writes the key set and gives it to each consumer through a channel that the consumer trusts. The key set has no signature. A consumer that reads a key set from the same place as the signed record gets no protection.

### Rotate and revoke a key

To rotate a key, do these steps:

1. Make a new signing key.
2. Add its public key to the key set with the status `active`. Change the status of the old key to `retired`.
3. Give the new key set to each consumer.
4. Start the Host with the new signing key.

To revoke a key, change its status to `revoked` and give the new key set to each consumer. A revocation applies to each record of the key, also to a record that the key signed before the revocation. A signed record has no trusted time, so a verifier cannot tell an earlier signature from a later one. The stored records stay in the store, and replay does not use the signature.

## Verification

The inputs are the bytes of one signed record document, and one key set. The verifier does the checks in this order and stops at the first refusal. The key set checks come first.

| Check | Refusal |
|---|---|
| K1. The key set is one strict JSON object. | `invalid_keyset`, rule `document` |
| K2. The key set has a `format`, and its major version is 1. | `unsupported_keyset_format` |
| K3. The `format` matches `entelos.keyset/<number>`. | `invalid_keyset`, rule `format` |
| K4. The key set agrees with its schema. | `invalid_keyset`, rule `schema` |
| K5. The algorithm of each key is `ed25519`. | `unsupported_algorithm` |
| K6. Each `public_key` is canonical base64 of 32 bytes. | `invalid_keyset`, rule `key_encoding` |
| K7. Each `key_id` is the key identifier of its `public_key`. | `invalid_keyset`, rule `key_id` |
| K8. No two keys have the same `key_id`. | `invalid_keyset`, rule `duplicate_key` |
| S1. The signed record is one strict JSON object. | `invalid_signed_record`, rule `document` |
| S2. The signed record has a `format`, and its major version is 1. | `unsupported_record_format` |
| S3. The `format` matches `entelos.record/<number>`. | `invalid_signed_record`, rule `format` |
| S4. The signed record agrees with its schema. | `invalid_signed_record`, rule `schema` |
| S5. The `algorithm` is `ed25519`. | `unsupported_algorithm` |
| S6. `record` is canonical base64. | `invalid_signed_record`, rule `record_encoding` |
| S7. `signature` is canonical base64 of 64 bytes. | `invalid_signed_record`, rule `signature_encoding` |
| S8. `record_ref` is the record reference of the record bytes and the record kind. | `record_mismatch`, rule `ref` |
| S9. The key set has the key of `key_id`. | `unknown_key` |
| S10. The status of the key is not `revoked`. | `revoked_key` |
| S11. The signature is valid for the public key and the byte string of the Signature section. | `signature_invalid` |
| S12. The record bytes are one strict JSON object. | `invalid_signed_record`, rule `record_json` |
| S13. The record has a `format`, and its major version is 1. | `unsupported_record_format` |
| S14. The `format` of the record matches `entelos.record/<number>`. | `invalid_signed_record`, rule `record_format` |
| S15. `scope` and `function` of the signed record are equal to those of the record. | `record_mismatch`, rule `binding` |

Strict JSON is UTF-8 text with no duplicate name in an object and no number that is not finite. The Go verifier also refuses an escape of a lone surrogate and a value that is nested deeper than the limit of its codec. The vectors do not cover these two cases. For K5 to K8, the verifier examines the keys in their order, and does the four checks on one key before the next key.

The record format rule is the rule of each stored record. A record without a `format`, and a record with a major version that the verifier does not know, fail closed with `unsupported_record_format`. This applies to the signed record (S2) and to the record in it (S13). A valid signature does not change this.

The codes have these meanings:

| Code | Meaning |
|---|---|
| `invalid_keyset` | The key set is malformed. The verifier accepts no record with it. |
| `unsupported_keyset_format` | The key set has no format, or a major version that the verifier does not know. |
| `unsupported_algorithm` | A key or a signed record names an algorithm that the verifier does not know. |
| `invalid_signed_record` | The signed record or the record in it is malformed. |
| `unsupported_record_format` | The signed record or the record in it has no format, or a major version that the verifier does not know. |
| `record_mismatch` | A field of the signed record does not agree with the record bytes. |
| `unknown_key` | The key set does not have the key that the signed record names. |
| `revoked_key` | The key that the signed record names is revoked. |
| `signature_invalid` | The signature is not valid for the record bytes, the record kind and the key. |

A refusal of the Go verifier is a diagnostic with one of these codes and the stage `signature`. If the code has rules, the message starts with the rule name.

On success, the verifier gives `record_kind`, `record_ref`, `scope`, `function`, `signer`, `key_id`, `key_status` and the record as a JSON object.

## Host

The embedding host installs one signing key in the Host. A request cannot select or change the key. Without a signing key, the Host writes no signed record, and its behavior does not change.

With a signing key, each write that stores a `decision`, `event` or `activation` record also stores one signed record of that record, in the same transaction. The state field `signatures` maps the record reference to the reference of the signed record. A record that has a signed record keeps it.

A Host with a signing key refuses a write that makes a `decision`, `event` or `activation` record of more than 512 KiB. It also refuses the write if the signed record has more than 1,000,000 bytes in its stored form, which a very long scope or function can cause. The refusal is `record_too_large` with the stage `signature`, and the write stores nothing. Thus each signed record fits in one protocol frame of 1 MiB, and the `record` operation can give it on each transport. A Host without a signing key has no such limit.

Before the Host stores a signed record, it does the checks of the Verification section on it, with its own public key. If a check fails, the result is `unsignable_record` with the stage `signature`, and the write stores nothing. Thus the Host never stores a signed record that the verifier refuses.

To get a signed record, do these steps:

1. Read the state with the `inspect` operation, and find the record reference in `signatures`.
2. Read the signed record with the `record` operation, with the kind `signed` and the reference from step 1.
3. Write the result to a file.

The commands `lifecycle-core` and `entelos-sidecar` take the signing key file with `--signing-key`. The file must be a regular file of the current user with the mode `0600`. The commands read a signing key file only on a Unix system. On each other system, `--signing-key` gives an error. `entelos-keygen` makes a new signing key file and prints its key set entry.

A signing key file is one JSON object with the fields `format` (`entelos.signing-key/1`), `algorithm` (`ed25519`), `key_id` and `seed`. `seed` is the 32 bytes of the Ed25519 private key seed in base64. Keep this file secret.

## Use by an outside consumer

An artifact store or a deploy controller can refuse a rule revision that has no valid approval or activation record. The consumer needs only the signed record file and its own copy of the key set. It makes no call to entelos.

To check a file with the reference command:

```sh
entelos-verify --keys keyset.json signed.json
```

The command prints one JSON line. On success, the line has `"status":"ok"` and the result, and the exit status is 0. On a refusal, the line has the code, and the exit status is 1. A consumer in a different language can do the checks of the Verification section with an Ed25519 library and a SHA-256 function. The vectors give test data for that code.

A valid signature shows that the holder of the private key signed these record bytes. It does not show these facts, so the consumer checks them:

- The record is of the necessary kind. For a deploy, examine `record_kind` and the `kind` field of an event.
- The record is for the correct `scope` and `function`, and for the revision that the consumer has. An activation record names the `snapshot_ref`. An approval record names the `snapshot_ref` and has the `disposition`.
- The record is the current selection. A later activation or suspension can replace it. Only the Host answers this question.
- The approval was correct. The signature states who signed, not that the review was good.

## Limits

- The signature has no trusted time. Thus a revocation applies to each record of the key.
- The key set has no signature and no expiry. Its integrity comes from the channel that gives it to the consumer.
- The Host signs only records that it writes after the embedding host installs a signing key. An earlier record has no signed record.
- The Host signs only `decision`, `event` and `activation` records. A receipt, a snapshot and a qualification report have no signature.
- The vectors do not cover the size limit of `record`. `TestSignedRecordFitsInOneFrame` covers it on the Go Host and the Go verifier.
- The Python Ed25519 code in [`ref_ed25519.py`](ref_ed25519.py) is for the public vectors only. It is slow and not constant-time.
- The test keys of the vectors are public. Never use them outside the vectors.
