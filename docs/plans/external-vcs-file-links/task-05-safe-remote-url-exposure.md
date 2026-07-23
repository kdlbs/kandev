---
id: "05-safe-remote-url-exposure"
title: "Safe remote URL exposure"
status: done
wave: remediation
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 05: Safe remote URL exposure

## Acceptance

- Generic repository create/update HTTP and service requests do not accept or persist a new `remote_url` clone target.
- The shared repository DTO continues to expose the already-persisted value read-only.
- Browser fixtures seed provider repositories through an existing provider/task path, not the generic settings API.
- Focused backend and desktop/mobile browser regressions pass.

## Output contract

Report RED/GREEN evidence, changed files, exact focused results, and any remaining clone-URL trust boundary.

## Completion report

- **Summary:** Replaced the rejected generic write-through with a read-only repository DTO contract and a trusted provider/task fixture path for browser setup.
- **Changed scope:** Repository DTO serialization and regression tests; generic repository handler/service request regression tests; E2E provider/task repository setup used by the desktop and mobile specs.
- **Focused verification:** `(cd apps/backend && go test ./internal/task/handlers ./internal/task/dto ./internal/task/service)` passed; the external-file-link desktop and mobile Playwright specs passed using provider/task-seeded repositories.
- **Trusted clone-URL boundary:** Generic repository create/update JSON ignores `remote_url`; only already-persisted values are exposed through the DTO. Provider/task resolution remains the trusted writer, so this work intentionally does not create a generic clone-target update API.
