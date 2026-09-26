---
id: "06-update-launcher-documentation"
title: "Update launcher documentation"
status: done
wave: 3
depends_on: ["01-cli-explicit-port-preflight", "02-native-launcher-port-preflight", "03-cli-health-ownership", "04-native-health-ownership", "05-cross-platform-address-in-use"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 06: Update launcher documentation

## Acceptance

- Public contributor, CLI, remote-cloud, and Windows-support docs describe that an occupied
  explicitly selected port is rejected before readiness and is never silently replaced.
- Documentation does not expose KANDEV_DESKTOP_HEALTH_TOKEN as a user configuration knob.
- The separate service-install KANDEV_SERVER_PORT limitation remains documented accurately and is
  not conflated with this repair.

## Verification

Update the smallest affected passages, then run:

~~~bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
~~~

## Files likely touched

- docs/public/contributing.md
- docs/public/cli.md
- docs/public/remote-cloud-environment.md
- docs/public/windows-support.md

## Dependencies

Tasks 01–05 establish the final launcher behavior and CI coverage that the public text must
describe.

## Parallelism

Sequential documentation pass after the implementation tasks. It is file-disjoint from source
work but intentionally waits for the final behavior wording.

## Inputs

- Spec sections: Explicit backend-port preflight, Windows address-in-use handling, and Out of
  scope.
- Plan section: Public documentation.
- Existing public pages identified by the docs-maintainer review.

## Risks

- Avoid promising a race-free port reservation; describe the launcher ownership/readiness check
  without exposing implementation details.
- Do not claim the service installer’s separate KANDEV_SERVER_PORT behavior was repaired.

## Completion

- Pages: contributing, CLI, remote-cloud, and Windows-support public guides were updated.
- Wording: explicit `run`/`start`/development port failures and Windows retry behavior are
  documented, while the service installer’s separate port semantics remain explicitly out of
  scope.
- Verification: public-doc validators pass, including the final CLI scope clarification.
