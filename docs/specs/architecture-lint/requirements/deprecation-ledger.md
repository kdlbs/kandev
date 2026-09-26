---
status: active
system: architecture-lint
created: 2026-09-26
updated: 2026-09-26
owners:
  - kandev
---

# Explicit deprecation tracking requirements

## Overview

The architecture-lint system checks repository source declarations and their
compatibility records. Contributors need each explicit deprecation to have an
owner and a removal condition. This is an internal repository-governance
contract. It does not change product behavior.

## Terminology

- **Declaration identity:** The stable source path, normalized declaration,
  and canonical deprecation marker reported by the scanner.
- **Compatibility entry:** The ledger record that names an owner, reason,
  introduction metadata, removal condition, and target.
- **Legacy baseline finding:** An existing unregistered annotation retained
  during the rollout.

## Requirements

### REQ-ARCHITECTURE-LINT-DEPRECATION-001: Track explicit API deprecations

**Intent:** Keep new deprecated declarations tied to accountable owners and
reviewable removal conditions without requiring immediate removal of existing
APIs.

#### Acceptance criteria

- **AC-ARCHITECTURE-LINT-DEPRECATION-001.1:** When a supported Go or TypeScript
  deprecation annotation is attached to a production declaration, the linter
  shall identify its exact path, declaration, and canonical marker. It shall
  ignore generated sources, tests, fixtures, third-party sources, ordinary
  prose, and strings.
- **AC-ARCHITECTURE-LINT-DEPRECATION-001.2:** When an annotation is not an exact
  legacy-baseline finding, the linter shall require a valid compatibility entry
  for that path, declaration, and marker. A missing or stale declaration
  registration shall fail validation.
- **AC-ARCHITECTURE-LINT-DEPRECATION-001.3:** When TypeScript functions or
  methods have overloads, their normalized generic and parameter signatures
  shall keep distinct declaration identities stable across source reordering
  and formatting. An indistinguishable repeated annotation identity shall fail
  as ambiguous.
- **AC-ARCHITECTURE-LINT-DEPRECATION-001.4:** When a deprecated declaration is
  removed, the linter shall reject its stale baseline finding or compatibility
  entry. After initial rollout, the rule baseline shall only shrink.

## Out of scope

- Product deprecation semantics or customer-facing migration policy.
- A global ban on deprecations or compatibility aliases.
- Automatic ledger or baseline changes.
- Immediate removal of the existing deprecated APIs.
