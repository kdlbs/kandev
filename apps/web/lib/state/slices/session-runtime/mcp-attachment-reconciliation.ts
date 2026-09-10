import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type {
  MCPAttachmentHistory,
  MCPAttachmentServer,
  MCPAttachmentStatus,
  MCPToolSummary,
} from "./types";

const MCP_ATTACHMENT_STATUS_VALUES: readonly MCPAttachmentStatus[] = [
  "unknown",
  "delivered",
  "connected",
  "active",
  "failed",
  "filtered",
  "unavailable",
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || value === null || typeof value === "string";
}

function isOptionalBoolean(value: unknown): boolean {
  return value === undefined || value === null || typeof value === "boolean";
}

function isOptionalNonNegativeInteger(value: unknown): boolean {
  return (
    value === undefined ||
    value === null ||
    (typeof value === "number" && Number.isInteger(value) && value >= 0)
  );
}

function isOptionalTimestamp(value: unknown): boolean {
  return (
    value === undefined ||
    value === null ||
    (typeof value === "string" && parseStrictRfc3339Timestamp(value) !== null)
  );
}

function isOptionalStringFields(
  value: Record<string, unknown>,
  fields: readonly string[],
): boolean {
  return fields.every((field) => isOptionalString(value[field]));
}

function isMcpAttachmentSource(value: unknown): boolean {
  return value === undefined || value === null || value === "kandev" || value === "profile";
}

function isMcpAttachmentStatus(value: unknown): value is MCPAttachmentStatus {
  return (
    typeof value === "string" && MCP_ATTACHMENT_STATUS_VALUES.includes(value as MCPAttachmentStatus)
  );
}

function isValidMcpToolSummary(value: unknown): value is MCPToolSummary {
  if (!isRecord(value) || !isNonEmptyString(value.name)) return false;
  return (
    isOptionalString(value.description) &&
    isOptionalBoolean(value.input_schema_truncated) &&
    isOptionalNonNegativeInteger(value.estimated_tokens)
  );
}

function isValidMcpAttachmentServerFields(value: Record<string, unknown>): boolean {
  return (
    isMcpAttachmentSource(value.source) &&
    isOptionalStringFields(value, [
      "transport",
      "target",
      "reason_code",
      "summary",
      "connection_id",
      "tool_token_estimator",
    ]) &&
    isOptionalTimestamp(value.tools_listed_at) &&
    isOptionalNonNegativeInteger(value.tool_count) &&
    isOptionalBoolean(value.tool_catalog_truncated) &&
    (value.tools === undefined ||
      value.tools === null ||
      (Array.isArray(value.tools) && value.tools.every(isValidMcpToolSummary)))
  );
}

function isValidMcpAttachmentServer(value: unknown): value is MCPAttachmentServer {
  return (
    isRecord(value) &&
    isNonEmptyString(value.name) &&
    isMcpAttachmentStatus(value.status) &&
    isValidMcpAttachmentServerFields(value)
  );
}

function isValidMcpAttachmentAttempt(value: unknown): value is MCPAttachmentHistory["current"] {
  if (!isRecord(value)) return false;
  if (!isNonEmptyString(value.attachment_attempt_id)) return false;
  if (
    typeof value.started_at !== "string" ||
    parseStrictRfc3339Timestamp(value.started_at) === null
  ) {
    return false;
  }
  if (!isOptionalTimestamp(value.updated_at)) return false;
  return (
    value.servers === undefined ||
    value.servers === null ||
    (Array.isArray(value.servers) && value.servers.every(isValidMcpAttachmentServer))
  );
}

function isValidMcpAttachmentHistory(value: unknown): value is MCPAttachmentHistory {
  if (!isRecord(value) || value.version !== 1 || !isValidMcpAttachmentAttempt(value.current)) {
    return false;
  }
  return (
    value.previous === undefined ||
    value.previous === null ||
    (Array.isArray(value.previous) && value.previous.every(isValidMcpAttachmentAttempt))
  );
}

/** Returns the validated persisted MCP projection, or null for unsupported data. */
export function readMcpAttachmentHistory(value: unknown): MCPAttachmentHistory | null {
  return isValidMcpAttachmentHistory(value) ? value : null;
}

function mcpHistoryFreshness(history: unknown): bigint | null {
  if (!isRecord(history) || !isRecord(history.current)) return null;
  const timestamp = history.current.updated_at ?? history.current.started_at;
  return typeof timestamp === "string" ? parseStrictRfc3339Timestamp(timestamp) : null;
}

/** Whether an incoming valid history is newer than the existing live evidence. */
export function shouldReplaceMcpAttachmentHistory(
  existing: unknown,
  incoming: MCPAttachmentHistory,
): boolean {
  if (!existing) return true;
  const incomingFreshness = mcpHistoryFreshness(incoming);
  if (incomingFreshness === null) return false;
  const existingFreshness = mcpHistoryFreshness(existing);
  return existingFreshness === null || incomingFreshness > existingFreshness;
}
