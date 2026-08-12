---
name: red-green-bug-fix
description: >-
  Bug-fix workflow: replication contract, failing regression test (RED), minimal
  fix (GREEN), refactor. Use when the user invokes @red-green-bug-fix or asks to
  reproduce issues, add regression tests, or fix regressions.
disable-model-invocation: true
---

# Red/green bug fix

**Load:** This skill is **not** auto-attached. Invoke **`@red-green-bug-fix`** when starting or fixing an issue so this workflow is in context.

Each fix adds a test that fails for the **same reason** as the reported bug, then a minimal production change makes it pass.

**Must:** Document replication and verify RED before any production fix. Do not weaken assertions or skip the buggy path to go green.

**Never:** Lock in buggy output as expected (characterization of wrong behavior). Change production to satisfy a test that fails on setup, mocks, or wrong expectations.

## Procedure

1. **Characterize** — Replication contract: Issue, Trigger, Expected (correct), Actual (bug), Scope; optional Out of scope and Failure signature after repro. If expected vs actual are not observables, stop and clarify with the user. **Extend an existing test** if one should have caught this (add cases to the existing `Test…` table; do not fork `TestFoo_nested` or a new `_test.go` file unless the subsystem is genuinely separate).
2. **Replicate** — Manual or smallest automated repro; record failure signature (error, status, return value, log). Stop if repro disagrees with the issue.
3. **RED** — Smallest test on the production path; name with issue ID and symptom; must fail; failure must match contract (not merely “test failed”). **Right-RED gate:** prod path (not setup/unused mock) · failure matches Actual · expected matches Expected (correct) · stable on re-run · would pass if bug removed. If gate fails, rewrite or delete the test—do not chase wrong RED in production. Run tests; report test name, command, brief failure quote. **STOP** — present RED report and wait for user review before production changes, unless the user explicitly asked for RED and GREEN in the same task.
4. **GREEN** — Minimal production fix; run full package/suite in scope; no unrelated failures. Proceed only after RED is approved or the task scope includes implementation.
5. **Refactor** — Cleanup under green tests only; update tests and contract if observables change.

**Untestable:** Skipped or documented placeholder test citing issue and contract; manual steps in PR; follow-up for automation—no silent fix without an anchor.

## RED phase — test placement (see `go-testing.mdc`)

When writing failing regression tests:

- Add cases to the **existing** `Test…` for the API under test; reuse `fn`, `input`, and teardown.
- Pair `_test.go` with its source file (`decode_test.go`, not `nested_test.go`).
- Prefer **inline anonymous structs** in each case; use a **function-scoped** named type only when shared by 3+ cases in that test or required for interfaces/embed syntax.
- Do **not** create shared fixture packages (`testfixtures`, `nestfixtures`) or duplicate types already in the file.
- Add a short comment above the case group documenting the replication contract when behavior is non-obvious.

## When RED is wrong (symptom → action)

- Compile/import failure in test → fix test, not production.
- Failure only in mock/fake → assert through exported API or prod boundary.
- Failure message ≠ issue → reconcile contract (trigger/expected).
- Intermittent pass → isolate; fix flake before GREEN.
- Passes before fix → wrong path or assertion; rewrite RED.
- Huge GREEN diff → narrow assertion to contract observable.
- GREEN but bug remains in field → strengthen assertion.
- GREEN by changing expected to actual → revert; expected is correct behavior.

## Anti-patterns

Wrong expected without manual validation · over-mocked path · wrong test altitude (unit vs integration) · flaky RED · weak assertion (e.g. any error) · false GREEN (skip branch, weaken assert, drop test) · golden/expected copied from buggy output · parallel test function for same API (`TestFoo_nested`) · shared fixture package only for test structs · production changes before RED is verified and reviewed.

## Stuck on cause (RED ok, GREEN unclear)

List hypotheses; order by probability × cost to falsify; timebox smallest proof/disproof; fix only when one matches RED mechanism. Do not rewrite large subsystems before RED pins what broke.

## Regression vs characterization

Reported bug: expected = **correct** per contract. Undocumented correct behavior: characterize only when not fixing a known defect. Legacy touch: characterize paths you change, then refactor under tests.
