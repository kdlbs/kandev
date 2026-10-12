export type CoordinatorProfileField = "agent" | "executor";

// Mirrors design/coordinators.md's Validation > Messages table. A status
// value the client does not recognize (a future backend addition) reads as
// the field's `missing` message rather than being silently dropped (B8).
export function coordinatorProfileStatusMessageKey(
  field: CoordinatorProfileField,
  status: string | undefined,
): string | null {
  if (!status || status === "ok") return null;
  if (field === "agent" && status === "passthrough") {
    return "coordinator:agentProfilePassthroughWarning";
  }
  return field === "agent"
    ? "coordinator:agentProfileMissingWarning"
    : "coordinator:executorProfileMissingWarning";
}
