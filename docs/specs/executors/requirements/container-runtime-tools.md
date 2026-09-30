---
status: active
system: executors
created: 2026-09-29
owners:
  - kandev
---

# Published Container Runtime Tools

## Overview

Operators running agents with the Local executor inside a published Kandev
container need the agent's basic operating-system tools to work under the normal
runtime user. The executor system owns this execution-environment contract.
The release system retains image publication and version-channel ownership;
the agent system retains provider invocation and permissions.

## Requirements

### REQ-EXECUTORS-CONTAINER-TOOLS-001: Available process inspection

**Intent:** Foreground agent commands can check their owning process without
being terminated because a required process-inspection utility is absent.

#### Acceptance criteria

- **AC-EXECUTORS-CONTAINER-TOOLS-001.1:** In both published base and universal
  Linux images, the normal unprivileged runtime user shall be able to run
  `ps -o ppid= -p "$$"` from Bash and obtain the live shell's parent process ID.
- **AC-EXECUTORS-CONTAINER-TOOLS-001.2:** When Droid runs the ordered foreground
  commands `pwd`, `ls -la`, `cd /tmp`, `printf 'execute-test\n'`, `date`, `id`,
  and `uname -srm` in an otherwise supported container environment, a missing
  process-inspection utility shall not cause SIGKILL. Successful commands shall
  retain their output and zero exit status; an intentional nonzero command
  shall retain its stderr and exit status.
- **AC-EXECUTORS-CONTAINER-TOOLS-001.3:** This tool availability shall require no
  root agent execution, additional container capabilities, relaxed security
  profiles, credential transfer, or permission-mode changes.

## Out of scope

- Guaranteeing third-party agent behavior when an operator or agent sandbox
  denies process inspection or overrides the executable search path.
- Installing tools on native Local or SSH hosts, or modifying custom executor
  images and already published immutable image digests.
- Changing ACP, Droid's supervisor, authentication, or agent permission policies.
- Requiring paid provider access for the automated image dependency check.

## Related documents

- [System design](../system-design/container-runtime-tools.md)
- [Implementation plan](../../../plans/droid-execute-runtime-dependency/plan.md)
