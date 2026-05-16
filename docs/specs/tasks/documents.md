---
status: shipped
created: 2026-04-29
owner: cfl
---

# Task Documents

## Why

Tasks currently support a single "plan" document via `task_plans` with revision history. But tasks often need multiple documents: a spec, a plan, implementation notes, review findings. The plan infrastructure (revisions, author tracking, revert) is solid but locked to one document per task. Generalizing plans into a multi-document system lets agents and users attach structured content to tasks without reinventing revision tracking.

## What

### Document model

- A task has zero or more documents, each identified by a unique **key** within the task (e.g. `spec`, `plan`, `notes`, `review-findings`).
- Each document has: key, type, title, content (markdown), author info, and revision history.
- Document types: `plan`, `spec`, `notes`, `review`, `attachment`, `custom`. Type is metadata for display — all documents have the same capabilities.
- The existing `task_plans` table is migrated to `task_documents`. The `task_plan_revisions` table becomes `task_document_revisions`. Existing plans become documents with `key=plan`, `type=plan`.

### Revision history

- Every update creates a new immutable revision (existing plan revision behavior, unchanged).
- Revisions track: revision number, author kind (agent/user), author name, content snapshot.
- Revert support: any prior revision can be restored (creates a new revision pointing back to the original).
- Coalesce merge: rapid successive updates by the same author within a short window are merged into the latest revision (existing behavior, unchanged).

### API

```
GET    /tasks/:id/documents                    → list all documents for a task
GET    /tasks/:id/documents/:key               → get document by key (latest content)
PUT    /tasks/:id/documents/:key               → create or update document (auto-revisions)
DELETE /tasks/:id/documents/:key               → delete document and all revisions
GET    /tasks/:id/documents/:key/revisions     → list revision history
POST   /tasks/:id/documents/:key/revisions/:revId/restore → restore from prior revision
```

### Agent access

- Agents create/read/update documents via `kandev doc create <task-id> <key> --type <type> --title <title>` with content from stdin or `--content` flag.
- `kandev doc read <task-id> <key>` outputs the latest content.
- `kandev doc list <task-id>` lists all documents for a task.
- The MCP handler exposes matching tools.

### UI

- Task detail shows a "Documents" section below the description.
- Each document rendered as a collapsible card with: type badge (PLAN, SPEC, etc.), title, revision count ("rev 3"), last updated timestamp.
- Clicking a document expands to show rendered markdown content.
- "New document" button opens a dialog with key, type, title, and content editor.
- Revision history accessible via a dropdown on each document card.

### Attachments

- Documents with `type=attachment` store binary files (images, PDFs, etc.) rather than markdown text.
- Attachment content is stored on disk (not in SQLite) under the runtime data directory: `<home>/data/attachments/<task-id>/<key>.<ext>`. This is separate from the workspace config directory (`<home>/workspaces/`) which is reserved for declarative config files that can be git-synced.
- The DB row stores metadata only: key, filename, mime type, size bytes, disk path.
- Upload via `POST /tasks/:id/documents/:key/upload` (multipart form). Max file size: 10MB.
- Download via `GET /tasks/:id/documents/:key/download` (streams the file).
- Attachments have no revision history — upload replaces the previous file.
- Agents upload via `kandev doc upload <task-id> <key> <filepath>`.

### Backward compatibility

- Existing plan MCP tools (`plan_create`, `plan_update`, `plan_get`, `plan_list_revisions`) continue to work unchanged — they map to the document with `key=plan`.
- Existing plan API endpoints (`GET/PUT/DELETE /tasks/:id/plan`, plan revision endpoints) continue to work as aliases to the document API with `key=plan`.
- `TaskPlan` and `TaskPlanRevision` Go types are preserved as type aliases to the new document types. Existing code that uses these types compiles without changes.
- The plan service methods (`WritePlanRevision`, `GetTaskPlan`, etc.) are preserved as wrappers around the document service.
- No breaking changes to any existing plan usage.

## Scenarios

- **GIVEN** a task with no documents, **WHEN** an agent runs `kandev doc create TASK-1 spec --type spec --title "Feature Spec"`, **THEN** a document with key "spec" is created at revision 1.

- **GIVEN** a task with a "plan" document at rev 2, **WHEN** the agent updates it, **THEN** rev 3 is created and the document shows "rev 3" in the UI.

- **GIVEN** a task with documents [spec, plan, notes], **WHEN** viewing the task detail, **THEN** all three appear in the Documents section with their type badges and latest revision info.

- **GIVEN** the existing plan API `PUT /tasks/:id/plan`, **WHEN** called, **THEN** it updates the document with `key=plan` (backward compatible).

- **GIVEN** a document at rev 5, **WHEN** the user clicks "Restore rev 2", **THEN** rev 6 is created with rev 2's content and `revert_of_revision_id` pointing to rev 2.

- **GIVEN** a task, **WHEN** an agent runs `kandev doc upload TASK-1 screenshot /tmp/bug.png`, **THEN** an attachment document is created with key "screenshot", the file is stored on disk, and the task detail shows it in the Documents section with a download link.

- **GIVEN** the existing MCP tool `plan_create`, **WHEN** called with a task ID and content, **THEN** it creates/updates the document with `key=plan` (backward compatible, no behavior change).

## Out of scope

- Real-time collaborative editing (one writer at a time, last-write-wins)
- Document templates (e.g. auto-populate spec template on creation)
- Cross-task document linking (documents belong to one task)
- Attachment revision history (attachments are replace-only, not versioned)
- Image/file preview rendering in the UI (download link only for now)
