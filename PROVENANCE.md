# Source provenance

The contract/lifecycle adapters and neutral examples are original reference code. The expression engine and authoring tools remain upstream projects. No engine implementation or model gateway is copied here.

- [CEL Go](https://github.com/cel-expr/cel-go), pinned by the module lockfiles, supplies parsing, checking and execution.
- [CEL author tools](https://github.com/cel-expr/skills/tree/5eac36c53cd8971fa1e5548374238183e0d8b302) supply environment/prompt/compile/evaluation commands. The runner fetches their public source; that repository and its binaries are not bundled.
- [Tekton's GitHub PR example](https://github.com/tektoncd/triggers/blob/d187d9b6398deef09c0ac9fa4d7c7d5a47202113/examples/v1beta1/github/github-eventlistener-interceptor.yaml) establishes the second workflow. Tekton already supplies production filtering and resource orchestration; no native Tekton runtime/cluster was executed in this reference.
- [Octokit webhook schemas](https://github.com/octokit/webhooks/blob/aa80c24a3f55354465b8b1438d33dd5602956d0d/payload-schemas/api.github.com/pull_request/ready_for_review.schema.json) describe the real event/field shapes. Full provider schema validation is not implemented here.

Policy text, expected rows and reduced event envelopes are invented examples. Defensive negative cases can represent invalid provider combinations. They are not customer policies, live webhook captures, adoption evidence or a claim of provider conformance.
