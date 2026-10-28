# Contributing

Open an issue in this repository with the concrete workflow, a small neutral example and the change in behavior. Keep application-specific source authority, policy and payloads separate from reusable mechanics.

Run `python3 -B verify.py --work "$TASK_WORK"` with a fresh work directory before submitting a change. Keep intended failures beside successful cases. Explain unsupported cases and which tests you actually ran. A dependency/profile/record change also needs an explicit identity and compatibility assessment.

Do not submit generated binaries, caches, local Git stores, credentials, customer material or raw private evaluation data. Preserve project copyright, licence and NOTICE attribution, plus upstream dependency licences and notices. Avoid unrelated framework, language or deployment work without a demonstrated integration need.

## Licence

Project source is licensed under [Apache-2.0](LICENSE). Unless explicitly stated otherwise, contributions intentionally submitted for inclusion use those terms, as described in section 5 of the licence. Keep the [project attribution](NOTICE) and [third-party notices](THIRD_PARTY.md). The [licence decision](docs/LICENSING.md) records the change from MIT.
