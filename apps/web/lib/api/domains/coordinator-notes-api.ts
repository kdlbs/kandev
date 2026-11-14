import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import { coordinatorPath } from "@/lib/api/domains/coordinator-api";

// Mirrors internal/coordinator/notes_models.go and notes_service.go.
export type NoteType = "user" | "feedback" | "project" | "reference";
export const NOTE_TYPES: readonly NoteType[] = ["user", "feedback", "project", "reference"];

export type NoteCitationKind = "turn" | "task" | "pr" | "file";
export const NOTE_CITATION_KINDS: readonly NoteCitationKind[] = ["turn", "task", "pr", "file"];

export type NoteCitation = {
  kind: NoteCitationKind;
  id?: string;
  repository_id?: string;
  number?: number;
  path?: string;
  sha?: string;
};

export type NoteProvenance = { kind: "human" | "model"; user_id?: string; model?: string };

export type NoteStatus =
  | "deleted"
  | "expired"
  | "stale"
  | "unavailable"
  | "not_validated"
  | "over_cap"
  | "live";

export type NoteReferrers = {
  proposals: { id: string; status: string }[];
  tasks: { id: string }[];
};

export type NoteView = {
  id: string;
  type: NoteType;
  text: string;
  citations: NoteCitation[];
  provenance: NoteProvenance;
  version: number;
  status: NoteStatus;
  created_at: string;
  expires_at: string;
  deleted_at?: string;
  use_count: number;
  last_validated_at?: string;
  last_attempt_at?: string;
  referenced_by?: NoteReferrers;
};

export type NoteVersion = {
  note_id: string;
  version: number;
  type: NoteType;
  text: string;
  citations: NoteCitation[];
  provenance: NoteProvenance;
  actor_id?: string;
  reverted_from?: number;
  created_at: string;
};

export type NoteDetail = NoteView & { versions: NoteVersion[] };

export type NoteReference = { proposal_id: string; status: string; note_version: number };

export type NoteDraft = { type: NoteType; text: string; citations: NoteCitation[] };

export type NoteErrorCode =
  | "invalid_type"
  | "invalid_text_length"
  | "invalid_citation"
  | "citation_unresolved"
  | "credential_detected"
  | "note_limit_reached"
  | "invalid_request"
  | "invalid_field"
  | "conflict"
  | "unavailable";

export type NoteConflictReason = "version" | "expired" | "deleted";

export type NoteFailure = {
  code: NoteErrorCode | "unknown";
  reason?: NoteConflictReason;
  field?: string;
  index?: number;
};

/** Reads the closed reason set out of a rejected note request. */
export function noteFailure(error: unknown): NoteFailure {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") {
    return { code: "unknown" };
  }
  const body = error.body as Record<string, unknown>;
  const code = typeof body.code === "string" ? (body.code as NoteErrorCode) : "unknown";
  return {
    code,
    reason: typeof body.reason === "string" ? (body.reason as NoteConflictReason) : undefined,
    field: typeof body.field === "string" ? body.field : undefined,
    index: typeof body.index === "number" ? body.index : undefined,
  };
}

const JSON_HEADERS = { "Content-Type": "application/json" };

function notePath(workspaceId: string, coordinatorId: string, suffix = ""): string {
  return coordinatorPath(workspaceId, coordinatorId, `/notes${suffix}`);
}

function send<T>(
  path: string,
  method: "POST" | "PUT",
  body: unknown,
  options?: ApiRequestOptions,
): Promise<T> {
  return fetchJson<T>(path, {
    ...options,
    init: {
      ...options?.init,
      method,
      headers: { ...JSON_HEADERS, ...options?.init?.headers },
      body: JSON.stringify(body),
    },
  });
}

export async function listNotes(
  workspaceId: string,
  coordinatorId: string,
  includeDeleted: boolean,
  options?: ApiRequestOptions,
): Promise<NoteView[]> {
  const query = includeDeleted ? "?include_deleted=true" : "";
  const res = await fetchJson<{ notes: NoteView[] }>(
    notePath(workspaceId, coordinatorId, query),
    options,
  );
  return res.notes;
}

export function getNote(
  workspaceId: string,
  coordinatorId: string,
  noteId: string,
  options?: ApiRequestOptions,
): Promise<NoteDetail> {
  return fetchJson<NoteDetail>(
    notePath(workspaceId, coordinatorId, `/${encodeURIComponent(noteId)}`),
    options,
  );
}

export async function getNoteReferences(
  workspaceId: string,
  coordinatorId: string,
  noteId: string,
  options?: ApiRequestOptions,
): Promise<NoteReference[]> {
  const res = await fetchJson<{ references: NoteReference[] }>(
    notePath(workspaceId, coordinatorId, `/${encodeURIComponent(noteId)}/references`),
    options,
  );
  return res.references;
}

export function createNote(
  workspaceId: string,
  coordinatorId: string,
  draft: NoteDraft,
  options?: ApiRequestOptions,
): Promise<NoteView> {
  return send(notePath(workspaceId, coordinatorId), "POST", draft, options);
}

export function editNote(
  workspaceId: string,
  coordinatorId: string,
  noteId: string,
  edit: NoteDraft & { version: number },
  options?: ApiRequestOptions,
): Promise<NoteView> {
  return send(
    notePath(workspaceId, coordinatorId, `/${encodeURIComponent(noteId)}`),
    "PUT",
    edit,
    options,
  );
}

export function revertNote(
  workspaceId: string,
  coordinatorId: string,
  noteId: string,
  revert: { target_version: number; version: number },
  options?: ApiRequestOptions,
): Promise<NoteView> {
  return send(
    notePath(workspaceId, coordinatorId, `/${encodeURIComponent(noteId)}/revert`),
    "POST",
    revert,
    options,
  );
}

export function deleteNote(
  workspaceId: string,
  coordinatorId: string,
  noteId: string,
  version: number,
  options?: ApiRequestOptions,
): Promise<NoteView> {
  return send(
    notePath(workspaceId, coordinatorId, `/${encodeURIComponent(noteId)}/delete`),
    "POST",
    { version },
    options,
  );
}
