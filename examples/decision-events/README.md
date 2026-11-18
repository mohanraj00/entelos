# Decision event reader

This example uses the real Go Host and a Git store. It cites three invented
facts from `sample-ledger`. A person approves them and corrects only the second
fact to `green`. The first read stops after two facts. The reader saves its
results and `next_cursor` in one file. It reads that file to resume at the third
fact. A second run reads no events and keeps the correction.

Use Go 1.27.2 or later. Set `TASK_WORK` to a local scratch directory outside
the checkout and cloud storage. From the repository root, run:

```sh
go -C examples/decision-events run . --work "$TASK_WORK/decision-events"
go -C examples/decision-events run . --work "$TASK_WORK/decision-events"
go -C examples/decision-events test ./...
```

The reader checks each event with `CheckDecisionEvent`. It joins the original
fact digest to its own values. It applies only human blind corrections. The
producer owns this choice. The export makes no learning choice.

The file `checkpoint.json` holds the cursor and the corrected values. The
reader replaces it after a successful page. On a refusal, it keeps the old
file. The directory `store.git` holds the Host records. Keep both paths to resume.
The example has one reader process. It does not coordinate concurrent readers
or guarantee a checkpoint after a power failure.

The application runner checks the manifest identity against
[expected.json](expected.json). This fixture is in the same commit as the
candidate. It does not prove independent qualification or fact correctness.
The Host contexts are trusted. The example has no login flow or signing key.
A blind mark records the review interface's statement. It cannot prove what
a person saw elsewhere. See [EVENTS.md](../../src/protocol/events/EVENTS.md).
