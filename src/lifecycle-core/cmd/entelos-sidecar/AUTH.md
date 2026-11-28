# Sidecar authentication

This table gives the authentication of each wire operation of the sidecar, on each transport ([#237](https://github.com/mohanraj00/entelos/issues/237)). `TestSidecarAuthMatrix` in [`../../protocol/auth_matrix_test.go`](../../protocol/auth_matrix_test.go) checks it.

## Transports

The sidecar checks the credential before it reads the request frame or the request body. Thus the check is the same for each operation and each protocol version. The Host gets no request from a caller without a valid credential.

| Transport | Mode | Credential | Refused credential |
|---|---|---|---|
| Unix socket | `--token-file` | The [token prelude](../../../protocol/PROTOCOL.md#unix-socket-authentication) with the operator token. The context file gives the actor and the roles. | No prelude, another token, or a token that a rotation replaced. |
| Unix socket | `--credentials-file` | The token prelude with the token of one principal of the file. | No prelude, a token that is not in the file, or a token that the operator removed from the file. |
| HTTP binding | `--http-local-tokens` | `Authorization: Bearer` with the token of one principal of `--credentials-file`. Loopback only. | No header, a token that is not in the file, or a token that the operator removed from the file. |
| HTTP binding | `--issuer-file` | `Authorization: Bearer` with a signed claim of [APPROVAL.md](../../../protocol/approval/APPROVAL.md#credentials). | No header, a claim with an unknown key or a bad signature, or a claim after its `exp`. |

A refused credential gets the status `unauthenticated` at the stage `authentication`, with `"protocol":"1.0"`. The HTTP status is 401. The socket closes the connection after the response. The sidecar reads the token file, the credentials file and the issuer file again for each request. Thus a rotation or a removal has an effect on the next request.

A static token has no expiry date. The operator removes it from the file to end it. A signed claim ends at its `exp`.

The static files of `--http-static` need no credential. They are public files, not operations. Each `POST` and each request to the API path goes to the credential check.

## Operations

After the credential check, the Host checks the role of the principal (stage `authority`). The role column is the role of the Host table in `../../host.go`. The test checks the operation names and the first versions against the loaded schemas. It does not check the roles.

| Operation | Versions | Access | Host role |
|---|---|---|---|
| `ack` | 1.0 to 1.3 | write | `worker` |
| `activate` | 1.0 to 1.3 | write | `reviewer` |
| `checkpoint` | 1.0 to 1.3 | write | `maintainer` |
| `claim` | 1.0 to 1.3 | write | `maintainer` |
| `decide` | 1.0 to 1.3 | write | `reviewer` |
| `describe` | 1.0 to 1.3 | read | `caller` |
| `ingest` | 1.0 to 1.3 | write | `worker` |
| `inspect` | 1.0 to 1.3 | read | `reviewer` |
| `invoke` | 1.0 to 1.3 | write | `caller` |
| `load` | 1.0 to 1.3 | write | `worker` |
| `observe` | 1.0 to 1.3 | write | `observer` |
| `plan` | 1.0 to 1.3 | write | `reviewer` |
| `propose` | 1.0 to 1.3 | write | `author` |
| `qualify` | 1.0 to 1.3 | write | `runner` |
| `receipt` | 1.0 to 1.3 | read | `reader` |
| `record` | 1.0 to 1.3 | read | `reviewer` |
| `replay` | 1.0 to 1.3 | write | `reader` |
| `resolve` | 1.0 to 1.3 | read | `reader` |
| `restate` | 1.0 to 1.3 | write | `reader` |
| `source` | 1.0 to 1.3 | write | `reviewer` |
| `suspend` | 1.0 to 1.3 | write | `reviewer` |
| `takeover` | 1.0 to 1.3 | write | `reviewer` |
| `proposal` | 1.1 to 1.3 | read | `reviewer` |
| `proposals` | 1.1 to 1.3 | read | `reviewer` |
| `decision_events` | 1.2 to 1.3 | read | `event_reader` |
| `approval` | 1.3 | read | `reader` |
| `evidence` | 1.3 | read | `reviewer` |
| `promotion` | 1.3 | read | `reviewer` |

A `write` operation can publish state. A `read` operation publishes no state. The Host-only operations, for example `policy_decide` and `policy_replay`, have no wire schema. The sidecar refuses them with `invalid_request` at the stage `protocol`, after the credential check.

## Test

For each transport mode, each operation and each version of the table, the test sends three requests: one with no credential, one with a wrong credential, and one with an ended credential (a removed token, a replaced operator token, or an expired claim). It checks for each request:

- the status `unauthenticated` at the stage `authentication`, and the HTTP status 401;
- that the Host got no request;
- that the files of the Git store did not change.

Then one request with a valid credential gets to the Host. This shows that the setup of the test is correct. The test does not show the behavior of a TLS front proxy or of a remote client.
