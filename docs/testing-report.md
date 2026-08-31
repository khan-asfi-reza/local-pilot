# Local Pilot (Shamsu): Testing Report

Khan Asfi Reza, 2321616042, asfi.reza.232@northsouth.edu

Fateh Tus Saad, 2321638642, fateh.saad.232@northsouth.edu

Bayzid Hasan Imon, 2321740042, bayzid.imon.232@northsouth.edu

Department of Electrical and Computer Engineering, NorthSouth University, Dhaka

Source code, evaluation manifests and recorded runs: https://github.com/khan-asfi-reza/local-pilot

The report is about how Local Pilot (Shamsu) is tested. Shamsu is an offline
coding agent that runs a small language model on the user's own machine, and the
model is allowed to edit real files, run shell commands and start servers. Two
things have to be verified here, so two methods are used. Evaluation measures
whether a small model can build the software a specification asks for, and
testing measures whether the harness around the model behaves correctly.
Evaluation is the main method, and the runs were automated with Claude and
opencode, so a full regression pass could be repeated after every change without
a person sitting through it. The report covers the strategy used for both
methods, the test cases written, the results with the recorded scores, and the
failures.

## 1 Testing Strategy used for Shamsu

A test checks a fact that must always hold, and it returns pass or fail. An
evaluation runs a whole task, scores the result against a rubric, and the score
is allowed to vary between runs. Most software only needs tests, but Shamsu needs
both, because the language model is part of the product and its output is not
deterministic.

Three properties of the system decided the approach. The agent writes to real
files and runs real commands, so a defect in a permission gate can destroy the
user's work. The model is small enough to run on ordinary laptops and therefore
unreliable, so correctness is defined as the harness detecting a bad result
rather than the model producing a good one. And three client surfaces, the
terminal, the browser Code IDE and the Telegram bot, drive the same agent over
the same project directory, so the contracts between them have to be tested as
well as the code inside them.

| Aspect | Evaluation | Tests |
| --- | --- | --- |
| Question answered | Does the product work | Is the harness correct |
| Subject | The model and the harness together | The harness only |
| Verdict | A score out of a rubric | Pass or fail |
| Needs a running model | yes | no |
| Runtime | minutes to hours | eleven seconds |
| Run when | after a harness change | on every change |

### 1.1 Evaluation driven testing

Evaluation driven testing means we change the harness, run a fixed ladder of
tasks, compare the scores with the previous run, and keep the change only if the
scores improved. It is the main method used in this project, because an assertion
cannot describe what a correct output is when a language model produces it, and
because the interesting defects are in the harness and not in the model. A model
that writes a file to the wrong path is a model problem, but a harness that
accepts that result and reports success is a harness problem, and only a full run
against a rubric shows the second kind.

The ladder is in [eval](https://github.com/khan-asfi-reza/local-pilot/tree/master/eval). It has thirteen specifications ordered by difficulty,
where level 1 is fixing one bug in one file, level 3 is a self-contained page or
a small command line program, and level 5 is a marketplace or a social app with a
database, an API and a frontend. Each specification has a manifest next to it,
and the manifest lists weighted checks that a program can evaluate.

| Check type | What it verifies |
| --- | --- |
| file_exists | A required file was produced |
| file_absent | A file that must not exist was not produced |
| file_unchanged | A file was left byte identical |
| mutated_only | Only the allowed files changed |
| cmd | A command exits with the expected code and prints the expected string |
| pytest | The produced test suite passes |
| judge | A subjective criterion, graded outside the runner |

A manifest for a level 1 task looks like this, and the whole set is in [eval/checks](https://github.com/khan-asfi-reza/local-pilot/tree/master/eval/checks).

```json
{
  "name": "fix-calc-bug",
  "difficulty": 1,
  "max_steps": 12,
  "grounding": { "action": "fix", "explicit_targets": ["calc.py"] },
  "checks": [
    { "id": "tests-pass",      "type": "cmd",            "run": "python3 test_calc.py",
      "expect_exit": 0, "stdout_contains": "all tests passed", "weight": 4 },
    { "id": "test-unchanged",  "type": "file_unchanged", "path": "test_calc.py",
      "weight": 3, "hard": true },
    { "id": "no-new-files",    "type": "mutated_only",   "allow": ["calc.py"],
      "weight": 3, "hard": true }
  ]
}
```

One attempt runs like this. The runner copies the seed fixture into a fresh
sandbox and commits it with git so a baseline exists, runs the agent headlessly
with the step cap from the manifest, and then executes the checks inside that
sandbox. The score is the achieved weight divided by the achievable weight, the
pass threshold is 0.80, and an attempt below the threshold is retried up to two
times in a new sandbox, so a retry never inherits a half finished project. Each
attempt appends one line to [memory-log.jsonl](https://github.com/khan-asfi-reza/local-pilot/blob/master/eval/reports/memory-log.jsonl) with the task name,
the attempt number, the score and the ids of the checks that failed, and the run
as a whole is written as a JSON report in the same folder.

Three decisions were made so the score is meaningful.

A check marked hard sets the score to zero instead of subtracting its weight. An
agent scored on a rubric will find the cheapest way to satisfy it, and the
cheapest way to make a failing test pass is to edit the test file, so the bug
fixing task marks `test-unchanged` and `no-new-files` as hard. Without that, an
agent that deleted the test would still collect four of the ten points.

Mutation is measured and not trusted. The runner reads `git status` after the run
and compares it with the allow list, ignoring agent state folders such as
`.pilot` and `node_modules`, so the set of changed files is taken from disk and
not from the summary the agent writes about itself.

Judgement is not done by the model under test. Checks of type judge are not
scored by the runner, they are written with their rubric and the produced file to
[judge-queue.jsonl](https://github.com/khan-asfi-reza/local-pilot/blob/master/eval/reports/judge-queue.jsonl) and graded separately, because a model of this
size is not a reliable judge of its own output. The landing page task is the
clearest example, since its objective checks only verify that the sections and
the anchor links exist, and whether the page is presentable is a judgement.

The output of the loop is a list of failure modes and not a single number. Most
guards in the harness exist because evaluation showed the model failing the same
way repeatedly, and each finding was turned into a deterministic rule, which was
then locked by a test.

| Failure found by evaluation | Rule added to the harness |
| --- | --- |
| Claims a file was created without calling a tool | Grounding gate, the run cannot finish until the named file exists |
| Edits a file the task never named | Drift message injected on the first wrong mutation |
| Repeats one tool call forever | Repeat breaker after six identical calls with no mutation |
| Rewrites a file it never read | Read before modify |
| Starts a dev server with a blocking command | Command refused and redirected to the serve tool |
| Long file write truncated by the backend | Output token limit set explicitly on every request |
| Builds a large project as one agent run and stalls | Top down decomposition into sub-tasks with a dependency graph |
| Hand writes boilerplate and gets the layout wrong | Deterministic scaffolders per language, run before the model |

### 1.2 A second evaluation set for complex projects

The thirteen ladder tasks are small enough to be scored by a manifest. Full stack
projects are not, because a rubric cannot say whether a Django and React
application actually works, so a second set was added in [eval/cmplx](https://github.com/khan-asfi-reza/local-pilot/tree/master/eval/cmplx) with ten
larger specifications: MediBook on FastAPI and React, LearnHub on Django and
React, TaskFlow on Express and React, DevForum on NestJS and React, Inventra on
Fastify and React, PollWave on Flask and Vue, ShipFast on Go and React, and the
OpenBazaar marketplace, which is also kept as the original docx so the document
reader is exercised as well.

These are evaluated by building them and then running them. The project is
generated, the dependencies are installed, the migrations run, the server is
started, and the frontend is built, and the result is a working application or a
list of reasons it did not start. `test/sandbox` holds one such build, a Django
backend with `manage.py`, a config package, a core app, its own virtualenv and a
test folder, next to a Vite, React and Tailwind frontend, with the REST contract
of fourteen endpoints saved as `apispec.json` and `openapi.yaml`. This set is
what produced the boot and repair pass in the orchestrator, because the failures
it exposed were not wrong files, they were correct looking files that did not
run.

### 1.3 Automated regression runs with Claude and opencode

A full pass over the ladder takes hours, because every attempt is a real agent
run on a local model, so it cannot be done by hand after every change. The
harness was therefore given non-interactive entry points from the beginning.
`pilot run --dir X --task "..." --format ndjson` performs one headless run and
emits every event as newline delimited JSON, and `pilot eval` runs the whole
ladder and writes the report. Both are ordinary commands with machine readable
output, so an external agent can drive them.

We used Claude and opencode as that driver, and the cycle it performs is below.

| Step | What the automation does |
| --- | --- |
| 1 | Run `pilot eval`, or a subset with `--only`, over the ladder |
| 2 | Read the JSON report and `memory-log.jsonl`, and list which checks failed on which task |
| 3 | Read the ndjson trace of the failing attempt and find the step where the run went wrong |
| 4 | Grade the queued judge checks against their rubrics and record a verdict |
| 5 | Write a summary of the failure classes seen in that pass |
| 6 | Propose and apply a change to the prompt, the harness or the architecture |
| 7 | Re-run the affected tasks and compare the scores with the previous pass |

The value of this is that a trace is long and repetitive and a person reading it
loses the pattern, while the same failure repeated across five tasks is obvious
to an agent that reads all five traces. Steps 3 and 5 produced most of the
findings, because a failure that looks like a model mistake in one trace usually
turns out to be a missing harness rule once the same shape appears in every
trace. The automation also runs `go test ./...` and the pytest suite before it
proposes a change and again after it applies one, so a change that improves a
score but breaks a permission gate is caught in the same cycle.

Two kinds of change came out of these runs. Prompt changes came first, because
they are cheap to try. The rule list in [prompt.json](https://github.com/khan-asfi-reza/local-pilot/blob/master/models/prompt.json) was rewritten from a
long list of prohibitions into a short prioritized list of positive instructions,
and it currently holds eleven rules, with a separate short list for chat mode and
per tool descriptions that can be tuned without rebuilding the binary. Rules were
added for the exact mistakes the traces showed, for example not rebuilding a file
that already exists, and writing one file at a time instead of announcing several
at once.

Architecture changes came when a prompt change stopped helping. The largest ones
are the top down orchestrator, which splits a specification into sub-tasks with a
dependency graph and runs them as separate child agent runs so no single call
sees the whole document, the deterministic language handlers in [harness/lang](https://github.com/khan-asfi-reza/local-pilot/tree/master/harness/lang),
which run the real generators and write the skeleton before the model is
involved, the contract first API builder, which designs the REST surface and
generates the plumbing so the model only fills the marked blanks, and the
evaluate and repair pass, which boots the produced application and repairs what
stops it from running. Each of those replaced work the model was doing badly with
work the harness does deterministically, which is the direction every evaluation
pass pushed the design in.

### 1.4 Test driven development

Test driven development was used, but it was not used from the beginning, and the
order matters here.

The structure was created first. The harness was built as a set of packages with
one direction of dependency, where events and model are at the bottom, then
tools, then lang and orchestrator, then agent on top, and the server and the
terminal are thin callers. The backend was divided the same way into core state,
services and routes. This shape was not designed by writing tests, it was
designed by building the system, seeing where the seams actually were, and moving
code until each package could be described in one sentence.

After the seams existed, they became the places where a test could be written
before the code, and from that point the work was test driven. The three
permission modes were written as failing tests before the gate logic was
finished, because the statement that plan mode changes nothing on disk is more
useful than the statement that a function returns an error string. The grounding
gate was written the same way, first a scripted model that replies "Done, I have
created area.c" without calling any tool, and then the loop was changed until
that run ended in a grounding failure. The path boundary, the session id
validator and the event contract shared by the three clients were written from a
table of hostile inputs prepared before the fix.

Two areas were kept out of this. Prompt text and skills are tuned by evaluation
instead of assertion, because a test that pins the wording of a prompt has to be
rewritten whenever the prompt improves. The deterministic scaffolders were
written first and pinned afterwards, because the correct shape of a Django or
FastAPI skeleton was found by running the real generators and not designed in
advance. The rule we followed is that anything answering whether an action may
happen is written test first, and anything answering what an output looks like is
pinned after it settles.

### 1.5 How the tests are set up

Every test drives the system the way a real caller does. The Go tests are in
their own package and can only reach the exported interface, so they use the same
entry points as the terminal and the server. The Python tests go through the real
FastAPI application, so the routing, the middleware and the dependencies are all
in the path.

No test needs a running model. Model calls and harness calls are answered by stub
servers that speak the same wire formats, which keeps the suite fast and
repeatable, and it also allows cases a real model cannot be asked to produce, for
example a reply that claims a file was written when no tool was called.

The tests do not touch the developer's machine. Projects are created in temporary
folders, and the shared data directory and the database are pinned to a temporary
home, so the real project registry, sessions and settings are never read or
written. A test for path traversal first writes a decoy file outside the project
and then checks the decoy still exists, because checking the error message alone
would pass even if the write had also gone through.

## 2 Test Cases

The suite has 193 cases, 80 in Go and 113 in Python, and it is in
[tests/go](https://github.com/khan-asfi-reza/local-pilot/tree/master/tests/go) and [tests/python](https://github.com/khan-asfi-reza/local-pilot/tree/master/tests/python).
The groups below are named by the property each one protects.

| Group | Cases | What is checked |
| --- | --- | --- |
| Permission gates and file boundary | 23 | Plan mode changes nothing and is not offered a mutating tool, ask mode pauses with a diff and honours a decline, auto mode never asks, a tool outside the allowed set is refused, a file cannot be rewritten before it is read, paths and commands reaching outside the project need approval, traversal is refused for reads and writes |
| Agent loop end to end | 6 | A full turn against a scripted model, a false completion is refused, plan mode stays read only, chat mode stays conversational, an unreachable backend reports an error, a repeating model is stopped |
| Orchestration and planning | 23 | A specification is split so no single call sees the whole document, planning retries on an unusable answer, invalid plans are rejected before anything runs, sub-tasks run in dependency order within a parallelism cap, shared memory between sub-tasks is scoped to dependencies and capped |
| Model registry and streaming | 16 | A broken registry fails at load, a remote model resolves to its own host, a tool call split across stream chunks is reassembled, two projects share one inference slot fairly and never hold it at once |
| Deterministic scaffolding | 6 | Stack detection from the prompt and from what is on disk, an existing project outranks the prompt, generated code parses, a second pass does not overwrite what the model filled in |
| Cross surface contracts | 6 | One folder maps to one project record across all three clients, the registry file matches the schema the other runtime reads, the event and diff payloads keep their field names |
| Sessions and project registry | 22 | Session ids are validated and cannot escape their folder, a thread saved by one surface is resumable by the others, a corrupt file does not break the listing, removing a project does not delete files |
| Trust boundary over HTTP | 12 | Full access routes refuse a caller from the network and a cross site page in the user's own browser, the sandboxed chat API stays reachable, reads and writes cannot escape the project root |
| Code IDE API | 20 | Opening and creating projects, rejecting invalid project names, creating and renaming and deleting entries, refusing to delete the project root, streaming a run and listing the thread it saved |
| Sandboxed chat API | 9 | Thread lifecycle and persistence, the chat path never requests file access, a harness outage is stored in the thread |
| Telegram pairing and state | 9 | Nothing is enabled until the owner enables it, a link code works once and expires, an unlinked chat is refused, each chat holds its own project and mode, the bot token is not in the public profile |
| Telegram runs and rendering | 29 | The polled snapshot reports progress, then an approval prompt with a diff, then the final reply, a decision reaches the harness waiting on it, a new message supersedes the previous run |
| Shared run path and concurrency | 12 | A run is mirrored to every watcher of the same project, two runs on one project stay separate threads, a slow watcher drops events instead of stalling the agent |

## 3 Results

### 3.1 Evaluation scores

The records below are every attempt in [memory-log.jsonl](https://github.com/khan-asfi-reza/local-pilot/blob/master/eval/reports/memory-log.jsonl), in the
order they were run. Fourteen attempts were made over ten of the thirteen
specifications in one regression session.

| No | Task | Difficulty | Attempt | Score | Passed | Checks that failed |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | remove-comments | 1 | 1 | 0.818 | yes | compiles |
| 2 | fix-calc-bug | 1 | 1 | 1.000 | yes | none |
| 3 | remove-comments | 1 | 1 | 1.000 | yes | none |
| 4 | rename-var | 1 | 1 | 1.000 | yes | none |
| 5 | palindrome-js | 2 | 1 | 1.000 | yes | none |
| 6 | stats-pylib | 2 | 1 | 1.000 | yes | none |
| 7 | wordcount-go | 2 | 1 | 1.000 | yes | none |
| 8 | landing-page | 3 | 1 | 0.714 | no | quality |
| 9 | landing-page | 3 | 1 | 0.714 | no | quality |
| 10 | landing-page | 3 | 2 | 0.714 | no | quality |
| 11 | landing-page | 3 | 1 | 1.000 | yes | none |
| 12 | snake-html | 3 | 1 | 1.000 | yes | none |
| 13 | todo-cli-py | 3 | 1 | 1.000 | yes | none |
| 14 | social-django-drf-react | 5 | 1 | 0.333 | no | manage, django-tests |

| Metric | Value |
| --- | --- |
| Specifications attempted | 10 of 13 |
| Attempts recorded | 14 |
| Attempts at or above the threshold | 10 |
| Mean score over all attempts | 0.878 |
| Mean of the best score per task | 0.933 |
| Tasks reaching a perfect score | 9 of 10 |

Read by difficulty, the best score for every level 1 and level 2 task is 1.000,
the best score for every level 3 task is 1.000, and the one level 5 task attempted
scored 0.333. The loss is therefore at the two ends. On `remove-comments` the
first attempt scored 0.818 because the C file it produced no longer compiled,
while both hard checks passed, which means the model removed every comment
without touching the decoy file and without creating anything new. On
`landing-page` the score stayed at 0.714 for three attempts and the only failing
check was `quality`, which the runner was scoring itself at that time, and after
the change described in section 1.1 the artifact goes to the judge queue instead
and the objective checks stand on their own, so the task scored 1.000. On
`social-django-drf-react` the failures were `manage` and `django-tests`, which
means the project was produced but did not run, and that is the failure class the
boot and repair pass was added for.

The complex set in [eval/cmplx](https://github.com/khan-asfi-reza/local-pilot/tree/master/eval/cmplx) has no numeric score. Its result is whether the
generated application installs, migrates, boots and serves, and the record of a
passing run is the build itself, for example the Django and React project kept in
`test/sandbox` with its fourteen endpoint contract.

### 3.2 Test results

All tests pass. The measurements were taken on macOS 15, Go 1.25 and CPython
3.14.7.

| Suite | Cases | Result | Wall clock |
| --- | --- | --- | --- |
| Go, harness | 80 | passed | 1.1 s |
| Python, backend and bot | 113 | passed | 10.2 s |
| Total | 193 | 193 passed, 0 failed | 11.3 s |

The tests written earlier next to the code they cover also pass, 75 Go unit tests
in 322 seconds and 2 backend integration tests in 1.8 seconds. One package
accounts for 307 of those 322 seconds because it runs real generators and package
installs, which is the reason the new suite was written to avoid that work.

### 3.3 Coverage

Coverage was measured as executed statements.

| Target | New suite alone | With everything |
| --- | --- | --- |
| Go harness packages | 40.4% | 54.7% |
| Python backend and bot | 50.3% | 50.3% |

The distribution is more informative than the totals. The parts that decide what
the agent may do are covered heavily, with the event contract at 100%, the
project registry at 87%, the model layer at 71% and the tool layer at 54%. The
low numbers have known reasons. The HTTP wrapper around the agent has no direct
test, the boot and repair pass needs npm, pip and a real server, the terminal
service needs a pseudo-terminal, and the React frontend has no automated tests.

## 4 Analysis of failures

### 4.1 Failures in the first test run

Ten runs failed before the suite passed. Eight were defects in the tests and two
were defects in the scaffolding the tests use. None of them was a defect in the
product, which is the expected result when tests are written against software
that already works, so each failure corrected an assumption and not a line of
product code.

| Cause | Count | Example |
| --- | --- | --- |
| Assertion too strong | 3 | The outline sent to the planner was expected to contain no section text, but it deliberately contains a one line preview of each section |
| Misunderstanding of the system | 3 | Planning was assumed to be a schema constrained call, but it is sent as an ordinary turn offering a single submit function, because schema decoding is expensive on this hardware |
| Defect in the test scaffolding | 2 | The stub harness closed its stream after sending an approval request, while the real harness keeps the connection open until the decision arrives |
| Wrong measurement | 2 | The coverage tool traced only the main thread while the test client runs the application on a worker thread, so every route handler was reported as unexecuted |

The last row also corrected the numbers in section 3.3. After thread tracing was
enabled, one route module moved from 1.7% to 69.8% with no change to the tests,
so a coverage number taken from a wrongly configured tool can be far below the
real value.

### 4.2 Failures found by evaluation

The failures the deterministic suite cannot find are the ones evaluation finds. A
model of this size fails in a small number of recognisable ways. It claims work
it did not do, it edits a file the task did not name, it repeats one action
without making progress, it rewrites a file it never read, it starts a server
with a command that blocks until the timeout, and it produces an edit or a
project that is correct in intent but does not compile or does not run.

The first five now end in an explicit refusal from the harness, and each refusal
has a test. The sixth is the one that is left, and it is visible in both imperfect
scores in section 3.1, the C file that does not compile at level 1 and the Django
project that does not run at level 5. The harness cannot improve the model, so
what it does instead is verify the result and repair it where it can, and what
the tests verify is that a result of this kind is not reported as a success.

### 4.3 Findings that remain open

| Finding | Severity |
| --- | --- |
| If the harness stream ends while a Telegram run is paused for approval, the run is reported as finished, so a dropped connection shows the user a completed run even though the action never ran | Medium |
| Database initialisation creates tables only for the models imported at that point, so an entry point that initialises early would create a partial database | Low |
| The evaluation report does not record which model produced it, so two reports can only be compared if the configuration is remembered separately | Low |
| The literal search fallback only helps a query that is invalid as a regular expression, so a query that is valid but was meant literally returns no matches | Low |
| Stack detection has no keyword for browser games and canvas applications, so those specifications skip the deterministic scaffold and use the slower model written one | Low |
| 134 deprecation warnings, almost all from two calls that are scheduled for removal, which will become errors on a future Python or FastAPI version | Low |

### 4.4 Limitations

The suite verifies the harness and not the generated software, so a passing run
does not mean an application produced by the agent works, and only evaluation can
answer that. The session reported here covers ten of the thirteen ladder
specifications, the three heaviest were not run in that pass, and evaluation is
run on demand rather than on a schedule, so the scores are a snapshot and not a
tracked series. The complex set is judged by whether the application runs, which
is a stronger result but not a number, so progress on it cannot be plotted. The
React frontend, the terminal service and the App Builder have no automated tests
because they need a browser, a pseudo-terminal and a running gateway. The
measurements were taken on macOS only, so the Windows code paths were checked by
hand.
