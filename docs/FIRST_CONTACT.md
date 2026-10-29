# First-contact sessions

This kit is for the exit test of milestone M1 ([issue #10](https://github.com/mohanraj00/entelos/issues/10)). An engineer who does not know the project tries the demo without help. The maintainer records if the demo completed, if the maintainer gave help, and each failure. Only the maintainer runs these sessions. An agent cannot complete them.

## Before the session

1. Merge the M1 wave 1 and wave 2 issues first. The engineer must see the README, the demo command and the walkthroughs.
2. Select an engineer who did not see the project before. Do not select a contributor.
3. Select one commit of `main` for all sessions of this round. Record it. Each engineer clones that commit.
4. Do not install toolchains or build the project for the engineer.

## Script

### What the maintainer says

Give the engineer the clone URL `https://github.com/mohanraj00/entelos.git` and the commit of the round. Then say only this text:

> Clone this repository at this commit and try to run the demo. Use only the repository and its docs, but do not read `docs/FIRST_CONTACT.md` or `.github/ISSUE_TEMPLATE/first-contact.yml`. They hold the rules for this session. Speak your thoughts while you work. I will not help, but I will write notes. You have 30 minutes. You can stop at any time.

At the end, ask these three questions in this sequence:

1. "In one sentence, what problem does this project solve?"
2. "What are the first three questions that you have now?"
3. "What did you do last?"

### What the maintainer does not say

- Do not explain the purpose, CEL, Git storage or the lifecycle.
- Do not give the demo command, a file name or a link, other than the clone URL, the commit and the two excluded paths in the text above.
- Do not answer questions during the session. Write each question down. Say "Please continue as if I am not here."
- Do not fix an error or a missing toolchain. Record it.
- Do not say the words "correction", "replay" or "restatement" before the engineer reads them.

### Words for the session end and a failure

In this kit, "stop" and "end" mean only the end of the session. A failure is a point before the demo completes where the engineer cannot continue alone. Examples are an error, a missing tool, a step that the engineer cannot do, help from the maintainer, and the session end before the demo completes. Record each failure, also if the engineer continues after it. The demo completes when `examples/quickstart/demo.py` prints `Result: passed` and exits with status 0.

### Time limit

The session is 30 minutes from the first command or page view. At 30 minutes, stop and ask the three end questions. If the engineer stops before 30 minutes, ask the questions at that time. Do not extend the session.

## Record

Open one issue for each session with the [first-contact form](https://github.com/mohanraj00/entelos/issues/new?template=first-contact.yml). The form source is [first-contact.yml](../.github/ISSUE_TEMPLATE/first-contact.yml). Write the record in the 24 hours after the session. The record has these fields:

- the commit of `main` that the engineer cloned;
- the round: the session number and the number of sessions selected for the round;
- the platform: operating system and CPU architecture;
- the toolchain: Go, Python and Git versions, or "not installed";
- the demo result: "completed" or "did not complete";
- the maintainer help: "no help" or "gave help";
- each failure in sequence, with the step and the help that the maintainer gave, or "None" if the demo completed without a failure;
- the answer to end question 3, as the engineer said it;
- the time to the session end, in minutes;
- the one-sentence problem statement, as the engineer said it;
- the check of that statement ("correct" or "not correct", see [Result](#result)) and its reason;
- the kit check: "did not open the kit files" or "opened a kit file";
- the questions that the engineer asked during the session, in sequence;
- the first three questions at the end, as the engineer said them, with "None" for each missing question;
- each error message, exactly as it occurred, or "None".

Milestone M2 ([issue #11](https://github.com/mohanraj00/entelos/issues/11)) uses the first three questions as its first input.

### Privacy

The record is public. Do not write names, employers, usernames, email addresses, home paths, hostnames or private data. Refer to the engineer as "Engineer A", "Engineer B" or "Engineer C". Remove user names and home paths from error messages. Do not attach screen recordings or audio.

## Result

A round is two or three sessions on the same commit of `main`. Select the number of sessions before the first session, and record it in each record. Run all of them, also after a failed session, so that the record of the commit is complete. The expected answer is set here, before the sessions. A problem statement is correct if it says both of these:

- rules that change inside a service, or the decisions of those rules;
- the earlier answers stay tied to their exact inputs and rule revisions, or a correction keeps the earlier answer.

The maintainer compares each statement with these two points and writes "correct" or "not correct" in the record, with the reason. The kit files are `docs/FIRST_CONTACT.md` and `.github/ISSUE_TEMPLATE/first-contact.yml`. If the engineer opened a kit file during the session, select "opened a kit file" in the kit check. Do not change the statement check for this reason.

### Why the expected answer stays in this file

Method: this file keeps the expected answer, and the maintainer tells the engineer not to read the two kit files. The maintainer watches the screen and records each view of these files in the kit check. The text names the two files, so the engineer knows which files to avoid.

Reason: the expected answer must be fixed in a commit before the first session, so that it is independent of the engineer statements. If the answer is kept outside the repository until the round ends, no commit shows that it was not changed after the sessions.

The exit test of M1 passes for a commit if each engineer of the round completes the demo without help, gives a correct problem statement and did not open a kit file. If one of these is not true for one engineer, the round fails for that commit. Keep each record of a failed round. A failure is evidence. If the docs change after a failed round, run a new round on the new commit with new engineers. Do not combine results from different commits. Do not run the same engineer again and call it a new first contact.
