# Security boundary

This is trusted local reference code. Repository writers and host context are trusted. The CLI is not a remote authentication or tenant-isolation gateway. Content identities and role labels are not cryptographic attestations.

An external adapter must authorize access to retained results and history, including retries after a permission change. The stored idempotency result is not a substitute for that access policy.

The Git store is a local directory of one operating system user. An account with write permission for the store directory is a repository writer. The update marker of the store has the mode `0600`, so that an account with only read permission cannot hold its lock and stop the writes. Use the mode `0700` for the store directory if no other account must read the records. See the [permissions of the Git store](src/lifecycle-core/README.md#permissions-of-the-git-store).

CEL receives explicit values and the pinned environment. It has no model, database, network or asynchronous host function here. The host process still has OS permissions. Cooperative cancellation and configured example limits are not a hard CPU/memory sandbox. Python ordinary-code examples are installed trusted code.

The sidecar checks a local bearer token before reading each request frame. The operator protects the token, socket, and their parent directories. Every authenticated client acts with the host context selected for that socket. Use distinct tokens for distinct contexts. The token provides no per-caller identity and does not protect against processes that can read the token or replace the socket. See the [sidecar setup](src/lifecycle-core/README.md#sidecar).

The optional HTTP binding of the approval API verifies an Ed25519 signed claim from one configured issuer, or a static token on loopback only. It binds to loopback by default and has no TLS. It is not a public internet boundary: it has no rate limit, no claim replay record and no revocation other than the issuer file. A network deployment needs a TLS front proxy that supplies these controls. See the [HTTP binding](src/protocol/approval/APPROVAL.md#http-binding).

The `ingest` operation records the statement of an executor. The Host checks the executor ID, the cursor sequence, the bundle and the build against the pinned snapshot, but it does not run the rule again. A content reference is not a signature, so these checks do not prove which executor made an entry. An application must trust the executor and must supply the expected module reference and the pin from a source that it trusts.

Use neutral data when reporting problems. For a possible vulnerability, use GitHub private vulnerability reporting when enabled. Otherwise report only that a private maintainer contact is needed, without posting sensitive payloads or exploit details. No response-time SLA is promised. A publisher must establish its reporting/contact route before claiming one.
