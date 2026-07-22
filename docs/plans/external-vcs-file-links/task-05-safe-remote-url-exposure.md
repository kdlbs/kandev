---
id: "05-safe-remote-url-exposure"
title: "Safe remote URL exposure"
status: done
wave: remediation
depends_on: ["04-repository-remote-url-contract"]
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
