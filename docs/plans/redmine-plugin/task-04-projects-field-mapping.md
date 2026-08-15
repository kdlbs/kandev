---
id: "04-projects-field-mapping"
title: "Project selection and field mapping"
status: not started
wave: 2
depends_on: ["02-plugin-repository-bootstrap"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 04: Projects and field mapping

## Intent

List and persist selected Redmine projects, and let the user map live statuses,
trackers, and priorities (plus custom fields) to Kandev concepts. Nothing about a
specific instance's status/tracker/priority names is ever hardcoded.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: project listing/pagination, field
mapping persistence, custom-field discovery with the admin/403 fallback.

## Dependencies

Task 02. Runs in parallel with tasks 03 and 05.

## Acceptance

1. Project listing walks `offset`/`limit=100` until `offset + len(items) >=
   total_count`; a 250-project instance returns all 250.
2. Selected project set persists and only issues in selected projects are polled by
   task 06/07's sync and watcher loops.
3. Field options (`/issue_statuses.json`, `/trackers.json`,
   `/enumerations/issue_priorities.json`) are fetched live; a grep across the plugin
   repo's source finds no literal status, tracker, or priority name.
4. Status→workflow-step, tracker→task-label, and priority→task-priority mappings
   persist, including the `isClosed` flag captured per status.
5. Custom fields come from `/custom_fields.json` when the key is admin; when that
   endpoint 403s, they are derived from the union of `custom_fields` observed on
   fetched issues, with a "derived from recent issues" note surfaced to the UI layer
   (task 08 renders it) rather than treated as an error.

## Verification

```sh
go test ./internal/projects/... ./internal/fieldmapping/... -race
grep -rniE '"(new|open|closed|resolved|rejected|feedback)"' internal/ && echo "FAIL: hardcoded status literal found" || echo "ok"
```

## Risks

The custom-fields 403 fallback is easy to get subtly wrong (e.g. silently dropping
fields instead of deriving them) — this was explicitly called out as a required
behavior in the original brief, not an edge case to skip.
