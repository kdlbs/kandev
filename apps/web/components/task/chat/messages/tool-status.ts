import type { ToolCallMetadata } from "../types";

export type NormalizedToolCallStatus = Exclude<ToolCallMetadata["status"], "in_progress">;

export function normalizeToolCallStatus(
  status: ToolCallMetadata["status"],
): NormalizedToolCallStatus {
  return status === "in_progress" ? "running" : status;
}
