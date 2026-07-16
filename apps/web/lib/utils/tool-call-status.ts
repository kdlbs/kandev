export type NormalizedToolCallStatus = "pending" | "running" | "complete" | "error" | "cancelled";

export function normalizeToolCallStatus(
  status: string | undefined,
): NormalizedToolCallStatus | undefined {
  if (status === "in_progress") return "running";
  if (status === "completed" || status === "success") return "complete";
  if (status === "failed") return "error";
  if (
    status === "pending" ||
    status === "running" ||
    status === "complete" ||
    status === "error" ||
    status === "cancelled"
  ) {
    return status;
  }
  return undefined;
}

export function isTerminalToolCallStatus(status: string | undefined) {
  const normalized = normalizeToolCallStatus(status);
  return normalized === "complete" || normalized === "error" || normalized === "cancelled";
}
