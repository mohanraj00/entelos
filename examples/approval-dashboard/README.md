# Approval dashboard

This example is a small standalone approval dashboard (issue [#157](https://github.com/mohanraj00/entelos/issues/157)). A reviewer sees the open proposals, the wait time, the evidence and the qualification result of each one. The reviewer records an accept or a reject verdict.

**The dashboard is optional. The [approval API](../../src/protocol/approval/APPROVAL.md) is the contract.** Any other client of the API can replace the dashboard. The dashboard has no state of its own. It reads and changes state only through the HTTP binding of the API.

## How it works

- The sidecar serves the page with `--http-static <directory>`. The page and the API thus have the same origin. The HTTP binding sends no CORS headers, so a page on another origin cannot read a response.
- The page sends each request to `POST /v1/request` with `Authorization: Bearer <token>`. In this example, the token is a local static token of `--credentials-file` (`--http-local-tokens`, loopback only).
- The page keeps the token in memory only. It does not store the token. A new page load needs the token again.
- The page uses the [mugham](https://github.com/mohanraj00/mugham) UI kit, release `v0.1.0`. [mugham.json](mugham.json) is the one pin: the repository, the tag and the commit. To change the kit version, change this file. `run.py` refuses a checkout if the tag or `HEAD` is not the pinned commit.
- The kit shows each value as text, never as markup. A proposal reason such as `<b>x</b>` shows as the characters `<b>x</b>`.

| Part | Kit component | Data |
|---|---|---|
| List | `card-grid` of `card`, each with 6 `fact` | The result of `proposals`, the oldest first. The facts include the wait time and the verdict status. The grid shows its empty state if no proposal is open. |
| Evidence | `card`, `fact` and `text` | The result of `proposal`: the snapshot, the source, each bundle text, each report and each verdict. |
| Verdict | two `action-button` (Accept, Reject) and one `editable-form` | The form keeps the reason in the page. It does not call the API. A button sends `decide` with the kept reason. It shows `Done` only after the API gives the verdict record. If the API refuses, the button shows `Not done` and the refusal code. |

The page shows the verdict kinds in different ways:

- **Human verdict.** A `decide` record. The card title is `Human verdict: accept` or `Human verdict: reject`, with the actor and the time.
- **Policy verdict.** A record of format `entelos.policy/1` ([POLICY.md](../../src/protocol/policy/POLICY.md)). The card title is `Policy verdict: <outcome>`, and the card says that it does not count as a human verdict. No Host writes this record yet, so only the unit cases of the browser test show it.
- **Stale.** A proposal with the report refusal `stale_qualification`, or a superseded snapshot. The fact `Verdict` starts with `Stale:`.

## Run it

`make verify` runs the browser test. To open the dashboard yourself, build the sidecar, `cel-handoff` and the record bundle as `verify.py` does. Then run:

```sh
python3 -B examples/approval-dashboard/run.py --work <new directory> --sidecar <entelos-sidecar> \
  --cel-binary <cel-handoff> --record-bundle <record-bundle> --serve
```

The script clones the kit at the pin, starts a sidecar on `127.0.0.1` and adds three synthetic proposals. It writes one line with the URL and the path of a private file (mode 0600) with the tokens of `reviewer-a` and `reviewer-b`. Open the URL and give one token. Stop the script with Ctrl-C.

## Browser test

Without `--serve`, `run.py` runs [test/harness.html](test/harness.html) in headless Chromium. The harness loads the dashboard in a frame on the same origin. It types, presses and reads the shown text as a person does:

1. An unknown token: the page shows the `unauthenticated` refusal.
2. The list: three cards, the oldest first, with the wait time and the verdict status of each.
3. The evidence view of the oldest proposal: the facts, the bundle text, the report and the reason as text.
4. One verdict: `reviewer-a` accepts. The button is `Pending` before the receipt and `Done` after it. The page then shows the new verdict record.
5. One API refusal: the same actor rejects the same snapshot. The API refuses with `duplicate_verdict`, and the Reject button shows it.
6. The list after the verdict: the accepted proposal is not open.

The expected values are hand-written from APPROVAL.md and the seed steps. No page output made them. After the browser exits, `run.py` reads the proposal as `reviewer-b` through the API. It checks that the verdict record exists and that its reference is the reference that the page showed.

The harness gets its job file from the site directory. That file holds a test token, so `run.py` adds it only in a test run, never with `--serve`.

## Limits

- The test runs in one browser: the Chromium of `ENTELOS_BROWSER` or a known path. The kit needs Chrome 123, Safari 17.2 or Firefox 138 or later.
- The page does not show the case inputs or the case outputs of a report. It shows the pass count.
- A bundle text longer than 10000 characters shows only its first 10000 characters, with the count of the omitted characters.
- The list shows at most 100 proposals. The API has no pagination yet.
- The page does not refresh by itself. Press `Back to the list` to read the list again.
- A local static token is for a local run only. For a remote reviewer, use signed claims and a TLS front proxy, as APPROVAL.md says.
