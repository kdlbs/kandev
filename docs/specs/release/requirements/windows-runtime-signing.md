---
status: active
system: release
created: 2026-10-10
owners:
  - kandev
---

# Windows Runtime Signing Requirements

## Overview

The release system owns the signatures and integrity of Windows runtime
archives consumed by package managers. Maintainers can configure SignPath
without making an unavailable signing service prevent release packaging.

## Requirements

### REQ-RELEASE-WINDOWS-SIGNING-001: Sign eligible Windows runtimes with a safe unsigned fallback

**Intent:** Deliver signed host binaries when signing completes while preserving
usable original binaries when it does not.

**User story:** As a release maintainer, I want signing before packaging so
archive checksums describe the binaries users receive.

#### Acceptance criteria

- **AC-RELEASE-WINDOWS-SIGNING-001.1:** Missing, empty, or whitespace-only token,
  organization, project, or policy configuration shall skip signing and report
  a notice without failing packaging.
- **AC-RELEASE-WINDOWS-SIGNING-001.2:** Scheduled and manual Nightlies shall never
  submit a signing request. Policies named `test` or beginning with `test-`,
  ignoring case, shall be accepted only for desktop validation and refused with
  a warning for Stable publication.
- **AC-RELEASE-WINDOWS-SIGNING-001.3:** Upload or signing failures, including
  incomplete extraction, shall leave both original unsigned host binaries
  intact and report a warning. Signing shall wait at most one hour for approval.
- **AC-RELEASE-WINDOWS-SIGNING-001.4:** Successful signing with both non-empty
  host binaries shall adopt the signed pair with executable modes before
  packaging. Missing or empty output shall retain the unsigned pair.
- **AC-RELEASE-WINDOWS-SIGNING-001.5:** Stable standard/full tar and ZIP archives
  shall retain the `kandev/bin` layout, helper-manifest contracts, and checksums
  of the adopted binaries. Nightlies remain full and npm-only; required artifact
  upload retries and publication success gates shall remain effective.

## Out of scope

- Changing desktop installer signing, certificate policy, or release-tag signing.
- Configuring credentials or SignPath projects and making real signing requests.
- Adding winget publication, altering release channels, or bypassing approval.
