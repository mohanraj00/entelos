# Third-party source and notices

This source-only reference does not vendor dependencies or distribute compiled binaries. The Go lockfiles select the runtime closure. The observed upstream license texts are retained below. They keep their own terms.

| Module | Version | Observed license | Retained text |
|---|---|---|---|
| cel.dev/cel-go | v0.32.0 | Apache-2.0 | [license](licenses/dependencies/cel.dev__cel-go__LICENSE.txt) |
| cel.dev/expr | v0.25.3 | Apache-2.0 | [license](licenses/dependencies/cel.dev__expr__LICENSE.txt) |
| github.com/antlr4-go/antlr/v4 | v4.13.1 | BSD-style; retained license digest identifies exact text | [license](licenses/dependencies/github.com__antlr4-go__antlr__v4__LICENSE.txt) |
| go.yaml.in/yaml/v3 | v3.0.4 | MIT and Apache-2.0 by file; see upstream LICENSE | [license](licenses/dependencies/go.yaml.in__yaml__v3__LICENSE.txt) |
| golang.org/x/exp | v0.0.0-20240823005443-9b4947da3948 | BSD-style; retained license digest identifies exact text | [license](licenses/dependencies/golang.org__x__exp__LICENSE.txt) |
| google.golang.org/genproto/googleapis/api | v0.0.0-20240826202546-f6391c0de4c7 | Apache-2.0 | [license](licenses/dependencies/google.golang.org__genproto__googleapis__api__LICENSE.txt) |
| google.golang.org/genproto/googleapis/rpc | v0.0.0-20240826202546-f6391c0de4c7 | Apache-2.0 | [license](licenses/dependencies/google.golang.org__genproto__googleapis__rpc__LICENSE.txt) |
| google.golang.org/protobuf | v1.36.10 | BSD-style; retained license digest identifies exact text | [license](licenses/dependencies/google.golang.org__protobuf__LICENSE.txt) |
| go.yaml.in/yaml/v3 | v3.0.4 | NOTICE attribution | [license](licenses/dependencies/go.yaml.in__yaml__v3__NOTICE.txt) |

The author CLI is fetched separately from its pinned public repository, which retains its Apache-2.0 license and dependency notices. Its source and binaries are not in this tree.

Before distributing any binary, enumerate its actual linked dependencies and retain required license/NOTICE/copyright material. That includes any optional tool binary selected for distribution. A source-only reference is not permission to omit upstream notices from a later binary release.

Python standard-library and system Git are prerequisites, not bundled software. The workflow/schema sources are credited in PROVENANCE.md; provider schemas and native Tekton code are not copied.
