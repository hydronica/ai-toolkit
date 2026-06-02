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

1. **Characterize** — Replication contract: Issue, Trigger, Expected (correct), Actual (bug), Scope; optional Out of scope and Failure signature after repro. If expected vs actual are not observables, stop and clarify with the user. Extend an existing test if one should have caught this.
2. **Replicate** — Manual or smallest automated repro; record failure signature (error, status, return value, log). Stop if repro disagrees with the issue.
3. **RED** — Smallest test on the production path; name with issue ID and symptom; must fail; failure must match contract (not merely “test failed”). **Right-RED gate:** prod path (not setup/unused mock) · failure matches Actual · expected matches Expected (correct) · stable on re-run · would pass if bug removed. If gate fails, rewrite or delete the test—do not chase wrong RED in production. Report test name, command, brief failure quote.
4. **GREEN** — Minimal production fix; run full package/suite in scope; no unrelated failures.
5. **Refactor** — Cleanup under green tests only; update tests and contract if observables change.

**Untestable:** Skipped or documented placeholder test citing issue and contract; manual steps in PR; follow-up for automation—no silent fix without an anchor.

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

Wrong expected without manual validation · over-mocked path · wrong test altitude (unit vs integration) · flaky RED · weak assertion (e.g. any error) · false GREEN (skip branch, weaken assert, drop test) · golden/expected copied from buggy output.

## Stuck on cause (RED ok, GREEN unclear)

List hypotheses; order by probability × cost to falsify; timebox smallest proof/disproof; fix only when one matches RED mechanism. Do not rewrite large subsystems before RED pins what broke.

## Regression vs characterization

Reported bug: expected = **correct** per contract. Undocumented correct behavior: characterize only when not fixing a known defect. Legacy touch: characterize paths you change, then refactor under tests.
