---
id: "05-issue-read-write-attachments"
title: "Issue read/write and attachments"
status: not started
wave: 2
depends_on: ["02-plugin-repository-bootstrap"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 05: Issue read/write and attachments

## Intent

Implement issue detail fetch, issue create/update, and the two-step attachment
upload-token flow.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: issue read/write client methods,
attachment upload flow.

## Dependencies

Task 02. Runs in parallel with tasks 03 and 04.

## Acceptance

1. `GET /issues.json` always sends `status_id=*`; a closed issue's update is not
   silently dropped.
2. `GET /issues/:id.json?include=journals,attachments,relations` returns full detail
   including journals/attachments/relations for read-only display.
3. `POST /issues.json` creates an issue with subject, description, project, tracker,
   status, priority, and custom-field values, returning its id and URL.
4. `PUT /issues/:id.json` updates an existing issue with the same field set.
5. Attaching a file performs `POST /uploads.json` (`Content-Type:
   application/octet-stream`), then includes the returned token in the issue payload's
   `uploads` array; the attachment appears on the created/updated issue.

## Verification

```sh
go test ./internal/issues/... -race
```

## Risks

None specific beyond the general REST-client correctness already covered by task 03's
client foundation; this task builds on top of it rather than duplicating auth/error
handling.
