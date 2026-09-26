---
status: draft
system: auth
created: 2026-05-21
owners:
  - Kandev
---
# Public Share Links (v0) Requirements

## Overview

When a user finishes a noteworthy task — an interesting bug-fix transcript, a clean refactor walkthrough, a teaching example — they have no way to send it to a teammate or post it externally without screenshotting message by message. A one-click "Share" that produces a stable public URL turns every completed task into a shareable artifact, drives word-of-mouth growth, and gives users a portable record of work they cared about.

## Requirements

### REQ-AUTH-PUBLIC-SHARE-LINKS-001: Public Share Links (v0)

**Intent:** When a user finishes a noteworthy task — an interesting bug-fix transcript, a clean refactor walkthrough, a teaching example — they have no way to send it to a teammate or post it externally without screenshotting message by message. A one-click "Share" that produces a stable public URL turns every completed task into a shareable artifact, drives word-of-mouth growth, and gives users a portable record of work they cared about.

#### Acceptance criteria

- **AC-AUTH-PUBLIC-SHARE-LINKS-001.1:** A "Share" button appears in the chat panel header for any session past the pre-history states (`CREATED` / `STARTING`). The button is hidden while the session is still warming up — there is nothing worth publishing yet — and visible for every other state (`RUNNING`, `IDLE`, `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, `CANCELLED`). Users can share an in-progress conversation if they want to; the backend mirrors this rule (see Failure modes).
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.2:** Clicking Share opens a dialog with a **mandatory preview-and-confirm step**: the dialog renders the redacted snapshot in a read-only viewer that reuses the existing session message components, plus a visible warning that anyone with the link will be able to view the conversation.
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.3:** The user only publishes by clicking an explicit confirm button in the dialog ("Publish to GitHub Gist"). The dialog never auto-publishes on open.
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.4:** On publish, kandev builds a **frozen snapshot** of the session and uploads it as a secret GitHub Gist on the user's authenticated GitHub account. The dialog then shows the gist URL with a "Copy" button.
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.5:** A snapshot is self-contained: it survives the underlying task, session, or workspace being deleted from kandev. It MUST NOT contain foreign keys to live kandev rows.
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.6:** Every snapshot passes through a redaction pass before being uploaded:
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.7:** Absolute paths under the session's worktree root are rewritten to repo-relative paths (`/Users/foo/proj/src/x.ts` → `src/x.ts`).
- **AC-AUTH-PUBLIC-SHARE-LINKS-001.8:** Known secret shapes are stripped: `sk-[a-zA-Z0-9]{20,}`, `ghp_[A-Za-z0-9]{36,}`, `gho_[A-Za-z0-9]{36,}`, `github_pat_[A-Za-z0-9_]{36,}`, `AKIA[0-9A-Z]{16}`.

## System design

The migrated technical source is split into [part 1](../system-design/public-share-links.md).
