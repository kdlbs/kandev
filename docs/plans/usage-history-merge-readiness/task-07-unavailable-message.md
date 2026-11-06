---
id: "07-unavailable-message"
title: "Name both remedies when retained usage is unavailable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-005
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-005.7
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 07: Name both remedies when retained usage is unavailable

## Summary

With the release toggle off on a current Host, the plugin tells users to upgrade Kandev, which is wrong. Change the message so it names both remedies. No logic changes.

## In scope

In `ui/bundle.js`, change these translation values in both `en` and `pt-pt`, and keep any `{{...}}` placeholders:
- `usageUpgrade`: "Retained usage isn't available. Update Kandev, or turn on Usage history in Settings > System > Feature Toggles. Previous saved usage is preserved."
- `importUnsupported`: "Retained usage isn't available. Update Kandev, or turn on Usage history in Settings > System > Feature Toggles."

Translate into European Portuguese for `pt-pt`. Don't use em dashes. Update the sentence about the toggle in the plugin `README.md`. Add an assertion to `test/bundle.test.mjs` that both locales' `usageUpgrade` and `importUnsupported` mention both remedies.

In `server/usage_history.go`, change `errUsageHistoryUnsupported` to "retained usage is unavailable; update Kandev or turn on the usage history toggle". Update tests that match the old text.

## Out of scope

- Showing live cost when the API is unavailable.

## Acceptance

- `make test` passes, and no plugin text tells users to only upgrade.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview). AC-COSTS-USAGE-IMPORT-005.7.

```text
| Retained usage isn't available. Update Kandev, or    |
| turn on Usage history in Settings > System > Feature |
| Toggles. Previous saved usage is preserved.          |
```

## Verification

```bash
(cd "$PLUGIN_ROOT" && go test ./server/... -count=1)
(cd "$PLUGIN_ROOT" && node --test test/bundle.test.mjs test/responsive-bundle.test.mjs)
(cd "$PLUGIN_ROOT" && make test)
```

`PLUGIN_ROOT` is `../kdlbs-kandev-plugin-session-cost-feature-investigate-token-co-9dc`. Run `git merge --ff-only origin/feature/investigate-token-co-9dc` there first, because that checkout was behind. Keep the sibling `kandev` checkout at the #3660 head.

## Files likely touched

- `ui/bundle.js`
- `server/usage_history.go`
- `test/bundle.test.mjs`
- `README.md`

## Dependencies

None.

## Risks

- Smoke helpers may assert the old message. Search `test/` for "Upgrade Kandev" and update the matches.

## Parallelism

`parallel-safe`

## Inputs

- [Import design](../../specs/costs/system-design/usage-history-import.md), Plugin messaging when retained usage is unavailable.

## Results

The composer and import card now name both remedies in English and European Portuguese. The server error and plugin README use the same guidance. Bundle tests assert both remedies in both locales.

The server tests, bundle and responsive-bundle tests, and full `make test` gate passed. The plugin checkout's `../kandev/pluginsdk` points to the newer #3660 SDK, which no longer has the legacy `SessionUsageWriteResult` type used by an existing test fixture. Validation used a temporary modfile pointed at the plugin's recorded SDK ref `9a751dd`; no dependency files changed.
