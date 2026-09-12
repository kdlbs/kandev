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
 * `unknown` is a rendered state, not a hidden one (design "Frontend state"):
 * while `unknown` with no record, the caller shows a "pause state
 * unavailable" affordance instead of a banner; while `unknown` with a record
 * already read, the banner stays and is marked stale.
 */
export type WorkspacePauseStatus = "unknown" | "known";

/**
 * The three things the slice holds per the design's "Frontend state"
 * section: the record or `null`, a status, and one monotonic counter plus
 * the sequence of the last update applied. `requestSeq` is bumped by
 * `beginPauseRequest` on every issued GET or POST; `appliedSeq` is the tag of
 * whichever response last passed both supersession guards.
 */
export type WorkspacePauseSliceState = {
  record: WorkspacePauseRecord | null;
  status: WorkspacePauseStatus;
  requestSeq: number;
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
