---
name: autotest
description: "Pre-commit/ship test runner: runs Go tests, analyses failures, fixes them in a loop until green."
---

# Auto Test

Run the bundled structured Go test helper as a closeout check. This is automated testing, not code review.

Uses `go test -json` under the hood with configurable modes. The helper parses JSON test events, reports structured results, and exits with a clear status code the agent can act on.

Use when:

- user asks for auto-test / run tests / test suite
- after code edits, before commit/ship
- after fixing a test failure, to verify the fix
- before merging a branch

## Contract

- Treat test failures as real bugs until proven otherwise. Never blindly update a test to make it pass without understanding why it broke.
- Verify every failure by reading the failing test, the code under test, and any relevant production code paths.
- When a test fails because the code changed intentionally, update the test to match the new behaviour — not the other way around.
- When a test fails because of a real regression, fix the production code.
- Prefer small, targeted fixes. No refactors unless the bug class demands it.
- When a fix changes code, re-run the affected package tests (at minimum) or the full suite.
- Keep going until the helper exits 0 with no failures while the work remains inside the original task scope.
- If a test fix triggers new failures, classify them before continuing: in-scope blocker vs. follow-up vs. stop-and-escalate.
- For flaky tests: add a code comment marking them as flaky, but do not skip or disable them without explicit approval.
- For missing tests on new code: flag the gap but do not block the PR on test coverage unless the change is high-risk (auth, crypto, data loss, concurrency).

## Skill Path (set once)

```bash
export AUTOTEST=".pi/skills/autotest/scripts/autotest"
```

## Pick Target

Full suite (all packages):

```bash
"$AUTOTEST" --mode full
```

Only packages with Go changes vs base:

```bash
"$AUTOTEST" --mode changed --base origin/main
```

Focused on specific packages:

```bash
"$AUTOTEST" --mode focused --package ./internal/ai --package ./internal/handler
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `full` | `full`, `changed`, or `focused` |
| `--base` | `HEAD~1` | Base ref for changed mode |
| `--package` | — | Package path for focused mode (repeatable) |
| `--race` | on | Enable Go race detector |
| `--no-race` | — | Disable race detector (faster, for quick cycles) |
| `--count` | `1` | Run count (`-count=N`) |
| `--timeout` | — | Test timeout (`-timeout=30s`, etc.) |
| `--json` | off | Output structured JSON instead of human-readable |
| `--verbose` | off | Show test output even on pass |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All tests passed |
| 1 | One or more tests failed |
| 2 | Build / compilation error |
| 3 | No tests found (no Go changes, or focused package has no tests) |

## Typical Workflow

1. Make code changes.
2. Run `"$AUTOTEST" --mode changed --no-race` for a fast check on affected packages.
3. If that passes, run `"$AUTOTEST" --mode full` (with race) before commit.
4. If tests fail, read the failing test and the code under test. Fix the bug or update the test.
5. Re-run step 2. Repeat until clean.
6. Commit.

### Parallel closeout

Format + test in parallel with autoreview:

```bash
# Terminal 1: autoreview
"$AUTOREVIEW" --mode local

# Terminal 2: autotest (fast cycle first)
"$AUTOTEST" --mode changed --no-race && "$AUTOTEST" --mode full
```

## Race Detector Trade-off

The race detector is important for concurrency bugs but makes the suite ~200× slower (~77s vs ~0.3s). Strategy:

- **During development**: `--no-race` for fast feedback
- **Before commit/ship**: full `--race` run
- **CI / automated**: always `--race`

## JSON Output

When `--json` is set, the helper prints a single JSON object to stdout. The agent can parse this to find exactly which tests failed without scanning raw test logs:

```json
{
  "packages": [
    {
      "package": "github.com/james/feedlot/internal/ai",
      "action": "pass",
      "elapsed": 0.001,
      "test_count": 15,
      "tests": [
        {"name": "TestSummarizeShort", "action": "pass", "elapsed": 0}
      ]
    }
  ],
  "summary": {
    "total": 15,
    "passed": 15,
    "failed": 0,
    "skipped": 0,
    "elapsed": 0.34
  },
  "build_error": "",
  "raw_returncode": 0
}
```

## Scope Governor

Auto-test is a closeout gate, not permission to rewrite the task.

Before fixing a failure, classify it:

- **In-scope blocker**: the failure is introduced by the current diff, affects the same owner boundary, and can be fixed without changing the task's contract.
- **Follow-up**: the failure is real but belongs to an adjacent bug class, sibling surface, or pre-existing flaky test.
- **Stop-and-escalate**: fixing the failure requires a design change, protocol change, or external dependency update that is outside the original request.

Stop and report the scope break instead of continuing when:

- a test fix turns into an architecture change;
- fixing one test breaks three others;
- two fix cycles haven't converged — pause and reclassify every remaining failure;
- the failure is pre-existing and unrelated to the current diff.

Do not keep committing speculative fixes just to satisfy the test runner.

## Final Report

Include:

- command used
- packages tested
- tests passed / failed / skipped
- any failures fixed and why
- the clean exit from the final helper run
