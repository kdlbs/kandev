---
status: active
system: prompt-sources
specification_version: 1
migration: complete
created: 2026-10-15
owners: [backend]
---

# Prompt sources

This system owns revisioned workflow definitions and their workspace instances.
It reconciles prompt-source input into existing native workflows and exposes
source locking, import preview and current drift. Workflow execution, task
movement, agent profile selection and external source assembly remain with
their existing systems.

- [Requirements](requirements/revisioned-workflows.md)
- [System design](system-design/revisioned-workflows.md)
- [Delivery plan](../../plans/prompt-sources/plan.md)
