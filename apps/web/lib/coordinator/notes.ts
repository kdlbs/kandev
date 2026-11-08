import type { TFunction } from "i18next";
import type {
  NoteCitation,
  NoteCitationKind,
  NoteFailure,
  NoteStatus,
  NoteType,
} from "@/lib/api/domains/coordinator-notes-api";

export const NOTE_TEXT_MAX = 500;
export const NOTE_CITATIONS_MAX = 10;

const TIME_FORMAT: Intl.DateTimeFormatOptions = { dateStyle: "medium", timeStyle: "short" };

export function formatNoteTime(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString(undefined, TIME_FORMAT);
}

export function noteTextLength(text: string): number {
  return Array.from(text.trim()).length;
}

export function noteStatusLabel(t: TFunction, status: NoteStatus): string {
  switch (status) {
    case "deleted":
      return t("coordinator:noteStatusDeleted");
    case "expired":
      return t("coordinator:noteStatusExpired");
    case "stale":
      return t("coordinator:noteStatusStale");
    case "unavailable":
      return t("coordinator:noteStatusUnavailable");
    case "not_validated":
      return t("coordinator:noteStatusNotValidated");
    case "over_cap":
      return t("coordinator:noteStatusOverCap");
    default:
      return t("coordinator:noteStatusLive");
  }
}

export function noteTypeLabel(t: TFunction, type: NoteType): string {
  switch (type) {
    case "user":
      return t("coordinator:noteTypeUser");
    case "feedback":
      return t("coordinator:noteTypeFeedback");
    case "project":
      return t("coordinator:noteTypeProject");
    default:
      return t("coordinator:noteTypeReference");
  }
}

export function citationKindLabel(t: TFunction, kind: NoteCitationKind): string {
  switch (kind) {
    case "turn":
      return t("coordinator:noteCitationTurn");
    case "task":
      return t("coordinator:noteCitationTask");
    case "pr":
      return t("coordinator:noteCitationPr");
    default:
      return t("coordinator:noteCitationFile");
  }
}

/** A one-line description of a typed citation, identifiers only. */
export function citationSummary(c: NoteCitation): string {
  switch (c.kind) {
    case "pr":
      return `${c.repository_id ?? ""}#${c.number ?? ""}`;
    case "file":
      return `${c.repository_id ?? ""}:${c.path ?? ""}@${(c.sha ?? "").slice(0, 12)}`;
    default:
      return c.id ?? "";
  }
}

/** Localized copy for a rejected request; the reason code never carries note content. */
export function noteFailureMessage(t: TFunction, failure: NoteFailure): string {
  switch (failure.code) {
    case "invalid_type":
      return t("coordinator:noteErrorInvalidType");
    case "invalid_text_length":
      return t("coordinator:noteErrorTextLength", { max: NOTE_TEXT_MAX });
    case "invalid_citation":
      return t("coordinator:noteErrorInvalidCitation");
    case "citation_unresolved":
      return t("coordinator:noteErrorCitationUnresolved");
    case "credential_detected":
      return t("coordinator:noteErrorCredential");
    case "note_limit_reached":
      return t("coordinator:noteErrorLimit");
    case "invalid_request":
    case "invalid_field":
      return t("coordinator:noteErrorInvalidRequest");
    case "unavailable":
      return t("coordinator:noteErrorUnavailable");
    case "conflict":
      return t("coordinator:noteErrorConflict");
    default:
      return t("coordinator:noteErrorUnknown");
  }
}

export function emptyCitation(): NoteCitation {
  return { kind: "task", id: "" };
}

/** Drops the fields that do not belong to the citation's kind and trims identifiers. */
export function normalizedCitation(c: NoteCitation): NoteCitation {
  switch (c.kind) {
    case "pr":
      return { kind: "pr", repository_id: (c.repository_id ?? "").trim(), number: c.number };
    case "file":
      return {
        kind: "file",
        repository_id: (c.repository_id ?? "").trim(),
        path: (c.path ?? "").trim(),
        sha: (c.sha ?? "").trim(),
      };
    default:
      return { kind: c.kind, id: (c.id ?? "").trim() };
  }
}

export function citationComplete(c: NoteCitation): boolean {
  switch (c.kind) {
    case "pr":
      return Boolean(c.repository_id?.trim()) && Number.isInteger(c.number) && (c.number ?? 0) > 0;
    case "file":
      return Boolean(c.repository_id?.trim() && c.path?.trim() && c.sha?.trim());
    default:
      return Boolean(c.id?.trim());
  }
}
