/**
 * Byte-exact allowlist mirroring the backend's `RoutineStatus.CanFire()`
 * (apps/backend/internal/office/models/enums.go): "active" and the empty
 * string (the column's "no writer set one" default) fire; "paused",
 * "archived", or any other value does not. No case folding, no trimming —
 * the comparison has to match the scheduler's exactly, or the UI promises a
 * fire the backend will not produce.
 */
export function isRoutineFiring(status: string): boolean {
  return status === "active" || status === "";
}
