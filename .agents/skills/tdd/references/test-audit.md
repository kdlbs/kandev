# Focused Test Audit

Use this procedure for a requested test-value audit, consolidation, or assertion repair.
Follow the test-value gate and anti-patterns in the parent TDD skill.
Optimize for confidence and maintenance cost, not deletion count.

## Read-only discovery

Choose one production owner and its test surface.
Read the complete candidate tests, production entry points, callers, related tests, scoped guidance, and relevant history.
Read the specifications that define the contract.
For dependency-backed claims, inspect the dependency source or types.
Check the test runner and CI routing before selecting a keeper.
Record a baseline result for every selected suite before editing.
Keep baseline failures separate from cleanup candidates.

Look for assertions with no independent oracle, mock-provided outcomes, source-text matches, or duplicated contracts.
Compare test names with their assertions.
Use these patterns as discovery clues, not automatic deletion rules.

## Candidate evidence

Record each candidate with one disposition:

| Mark | Action | Required evidence |
| --- | --- | --- |
| R | Retain | Name the contract and credible regression. |
| F | Repair the assertion | Name the claimed behavior that the assertion cannot prove. |
| C | Consolidate | Name the keeper and every assertion that moves there. |
| D | Delete | Name the remaining proof, or explain why no contract exists. |

Before a consolidation or deletion, record:

- The exact test name and location.
- The failure that the current assertion can detect.
- Production callers of any covered test-only seam.
- The keeper, its distinct risks, and any contract it cannot reach.
- Relevant history and the reason the test or seam exists.
- Any production or support code that becomes unnecessary.
- The risk and focused validation command.

If evidence is incomplete, retain the candidate until the uncertainty is resolved.

## Retention rules

Keep independent API, protocol, storage, migration, security, platform, package, release, and architecture contracts.
Keep shared fixtures that detect disagreement between Go and TypeScript implementations.
Keep call-order assertions when order changes observable behavior.
Keep performance instrumentation that detects a named regression without a reliable timing threshold.
Keep source inspection when it independently protects a required key, byte, path, or architecture boundary.
Prefer assertions that survive harmless identifier changes and source reorganization.
An exact implementation string needs stronger justification than a contract-level source check.

A failing retained test can expose a product defect.
Reproduce the failure and report it separately.
Do not delete the test or expand cleanup into an unrelated product fix.
Follow `/fix` for behavior changes that need a design package.

## Edit and prove

Choose one coherent batch after the evidence is complete.
Move each distinct regression into its keeper before deleting its old suite.
Consolidate setup without hiding the scenario.
Remove an obsolete test-only seam only after checking every production and test caller.
Do not remove instrumentation solely because its name contains `test`.
Update explicit test inventories or CI routing when a moved suite requires it.

For an uncertain or repaired assertion, make one deliberate mutation to the production owner.
Choose a mutation that breaks the named contract, not compilation or imports.
Run the keeper and check that its intended assertion fails.
Then restore the original source byte for byte, including pre-existing user edits.
Use a saved file copy and guaranteed cleanup for temporary mutations.
If the mutant passes, repair the assertion before claiming coverage.
Run mutations sequentially and never edit inputs while a test command is active.
Mutation proof supplements the pre-fix RED required for a product bug.

Use the existing targeted Go, Vitest, or Playwright commands from `/tdd` and `/e2e`.
Run the keeper and affected siblings after the final edit and source restoration.
Follow Kandev's worker limits and single-session delegation policy.
Do not add remote compute, independent review, or publication steps automatically.

## Handoff

Report repaired assertions, consolidated contracts, deleted seams, and valuable false positives.
Include baseline and final results, mutation outcomes, and unresolved evidence gaps.
Count production changes separately from tests and support code.
