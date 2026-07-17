---
id: "04-lifecycle-profile-ownership"
title: "Lifecycle profile ownership"
status: done
wave: 3
depends_on: ["02-resolution-and-routing-persistence", "03-dual-id-session-plumbing"]
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 04: Lifecycle profile ownership

**Acceptance:** runtime uses the execution profile for CLI/provider, credentials/env, model, mode, config options, CLI flags and permissions, passthrough, and MCP; it uses the Office identity for instructions, skills, Office JWT/capabilities, budgets, ownership, and executor preference; Kandev-owned identity env cannot be overridden.

**TDD cases:** Codex identity with Claude execution profile launches Claude config only; execution-profile MCP/config options/passthrough/permissions apply; Office skills/instructions and capability claims remain; environment precedence protects `KANDEV_*`; non-Office launch remains unchanged.

**Likely files:** lifecycle launch/profile/environment/passthrough/MCP/skill deployment paths, executor Office preparation, Office runtime token/capability code, and integration tests around the assembled launch request and command.

**Verification:** focused lifecycle and executor tests, including real profile resolution through the settings repository.

**Dependencies:** Tasks 02 and 03.

**Output contract:** enumerate every field by owner, tests proving no cross-profile leakage, files changed, and residual runtime risks.
