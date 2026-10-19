// Split out from types.ts to respect the 600-line file cap, mirroring
// routing-types.ts.

/** Mirrors the backend's `pause` object in the GET/POST `.../pause` and
 * `.../resume` responses. Absent (`null`) while the workspace is running. */
export type WorkspacePauseRecord = {
  id: string;
  reason: string;
  createdBy: string;
  createdByKind: string;
  createdAt: string;
};

/**
 * `unknown` means no read has been issued yet for the selected workspace.
 * `loading` means a read is in flight. `known` means the last read for the
 * selected workspace succeeded. `error` means the last read failed. Both
 * `unknown` and `loading` with no record render a reading indicator, never
 * the failure warning; only `error` with no record renders the unavailable
 * affordance, so a remount re-read cannot flash a warning. While `error`
 * or `loading` with a record already read, the banner stays; it is marked
 * stale only on `error`.
 */
export type WorkspacePauseStatus = "unknown" | "loading" | "known" | "error";

/**
 * The four things the slice holds per the design's "Frontend state"
 * section: the record or `null`, a status, the workspace the record and
 * status were read for, and one monotonic counter plus the sequence of
 * the last update applied. `requestSeq` is bumped by
 * `beginPauseRequest` on every issued GET or POST; `readSeq` tracks the latest
 * GET, and `appliedSeq` is the tag of the last accepted success or read failure.
 * A failed mutation does not supersede an outstanding read. `workspaceId`
 * lets a remount tell a same-workspace re-read (keep the record) from a
 * workspace change (clear it) without relying on hook-instance memory.
 */
export type WorkspacePauseSliceState = {
  record: WorkspacePauseRecord | null;
  status: WorkspacePauseStatus;
  workspaceId: string | null;
  requestSeq: number;
  readSeq: number;
  appliedSeq: number;
};

/**
 * Discriminates the four table rows `applyPauseResponse` implements. A
 * "read" outcome comes from `GET .../pause`; a "mutate" outcome comes from
 * `POST .../pause` or `POST .../resume`. Success carries the server's
 * reported paused state and record; failure carries neither, since -006.6's
 * "keep the displayed state matching the server's" means nothing here is
 * ever applied optimistically.
 */
export type WorkspacePauseOutcome =
  | { kind: "read-success"; paused: boolean; record: WorkspacePauseRecord | null }
  | { kind: "read-failure" }
  | { kind: "mutate-success"; paused: boolean; record: WorkspacePauseRecord | null }
  | { kind: "mutate-failure" };
