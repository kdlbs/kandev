---
id: "04-repository-remote-url-contract"
title: "Repository remote URL HTTP contract"
status: superseded
wave: 3
depends_on: ["01-link-foundation", "02-toolbar-wiring"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 04: Repository remote URL HTTP contract

## Historical supersession

This task is superseded by [task 05](task-05-safe-remote-url-exposure.md). Security review rejected its proposed generic `remote_url` create/update write-through because that field is a clone target and must not become a broad user-controlled input.

There is no active acceptance, output, or verification contract for this task. Its safe read-only DTO portion continues through task 05: already-persisted `remote_url` is serialized for consumers, while generic repository create/update requests ignore submitted clone URLs. Provider/task resolution remains the trusted source of clone-target data.
