import {
  isEntityReference,
  normalizeEntityReferences,
} from "@/lib/entity-references/message-references";
import type { EntityReference } from "@/lib/types/entity-reference";

export type QueuedGhostEditDraft = {
  content: string;
  references: EntityReference[];
};

// i18n-exempt: this is an internal sessionStorage key, not user-facing copy.
const QUEUED_EDIT_DRAFT_PREFIX = "kandev:queued-edit-draft:";

function storageKey(sessionId: string, entryId: string): string {
  return `${QUEUED_EDIT_DRAFT_PREFIX}${encodeURIComponent(sessionId)}:${encodeURIComponent(entryId)}`;
}

function sessionStorageOrNull(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
}

export function readQueuedGhostEditDraft(
  sessionId: string,
  entryId: string,
): QueuedGhostEditDraft | null {
  const storage = sessionStorageOrNull();
  if (!storage || !sessionId || !entryId) return null;
  try {
    const raw = storage.getItem(storageKey(sessionId, entryId));
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    const draft = parsed as { content?: unknown; references?: unknown };
    if (
      typeof draft.content !== "string" ||
      !Array.isArray(draft.references) ||
      !draft.references.every(isEntityReference)
    ) {
      return null;
    }
    return {
      content: draft.content,
      references: normalizeEntityReferences(draft.references),
    };
  } catch {
    return null;
  }
}

export function writeQueuedGhostEditDraft(
  sessionId: string,
  entryId: string,
  draft: QueuedGhostEditDraft,
): void {
  const storage = sessionStorageOrNull();
  if (!storage || !sessionId || !entryId) return;
  try {
    storage.setItem(storageKey(sessionId, entryId), JSON.stringify(draft));
  } catch {
    // Storage can be disabled or full. The live editor still owns this draft.
  }
}

export function clearQueuedGhostEditDraft(sessionId: string, entryId: string): void {
  const storage = sessionStorageOrNull();
  if (!storage || !sessionId || !entryId) return;
  try {
    storage.removeItem(storageKey(sessionId, entryId));
  } catch {
    // The draft is already cleared from the active editor.
  }
}
