---
id: "03-startup-proxy-guidance"
title: "Document startup and proxy recovery"
status: done
wave: 3
depends_on:
  - "01-bind-aware-readiness"
  - "02-actionable-startup-failures"
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-STARTUP-003
acceptance_criteria:
  - AC-LAUNCHER-STARTUP-003.1
  - AC-LAUNCHER-STARTUP-003.2
  - AC-LAUNCHER-STARTUP-003.3
system_design:
  - ../../specs/launcher/system-design/startup-recovery.md
---

# Task 03: Document Startup and Proxy Recovery

## Summary

Update the public CLI and configuration guides after the launcher behavior is
implemented. Give operators copyable exact-IP and CIDR examples and explain
which log messages are unrelated to readiness.

## In scope

- Document bind-aware readiness and the new failure summary.
- Explain early exit compared with launcher-initiated shutdown.
- Show exact-IP and narrow-CIDR trusted-proxy examples.
- Explain that `peer` identifies the immediate proxy address.
- State the security effect of trusting a CIDR.

## Out of scope

- A new documentation page or navigation entry.
- Deployment-specific Traefik or Kubernetes manifests.
- Automatic proxy discovery.

## Acceptance

- The CLI guide gives a short recovery procedure for every launcher failure
  class.
- The configuration guide shows stable-proxy and dynamic-proxy-network YAML.
- The guidance does not recommend broad private-network trust as a default.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/cli.md`
- `docs/public/configuration.md`

## Dependencies

Tasks 01 and 02 define the shipped behavior and exact output.

## Risks

- The examples must not imply that browser-client networks are proxy networks.

## Parallelism

`sequential`

## Inputs

- `REQ-LAUNCHER-STARTUP-003` and the proxy-guidance design.
- Existing CLI troubleshooting and trusted-proxy reference sections.

## Results

Updated the public CLI and configuration guides with bind-aware readiness,
startup failure recovery classes, the distinction between readiness and ACP or
forwarded-host messages, exact-IP and narrow-CIDR trusted-proxy examples, and
the immediate proxy-peer security rule.

Verification passed:

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/lint-spec-files.py --all`
