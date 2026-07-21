import type { PermissionActionDetails } from "./use-permission-handlers";

const PREFERRED_RAW_INPUT_KEYS = [
  "command",
  "cmd",
  "file_path",
  "filePath",
  "path",
  "url",
  "query",
  "pattern",
];

const MAX_LEN = 200;

function truncate(value: string): string {
  if (value.length <= MAX_LEN) return value;
  return `${value.slice(0, MAX_LEN - 1)}…`;
}

function stringifyValue(value: unknown): string | null {
  if (value === null || value === undefined) return null;
  if (typeof value === "string") return value.trim() || null;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return null;
  }
}

function pickFromRawInput(raw: Record<string, unknown>): string | null {
  for (const key of PREFERRED_RAW_INPUT_KEYS) {
    if (key in raw) {
      const v = stringifyValue(raw[key]);
      if (v) return v;
    }
  }
  const v = stringifyValue(raw);
  return v && v !== "{}" ? v : null;
}

/**
 * One-line human-readable summary of what the agent is asking permission to do,
 * derived from action_details. Returns null when no useful detail is available
 * (so the caller can skip rendering).
 *
 * Priority: explicit description > raw_input (best key) > legacy top-level
 * command/path. Skipped when the result duplicates `title` so we don't show
 * the same string twice.
 */
export function summarizePermissionAction(
  details: PermissionActionDetails | undefined,
  title: string,
): string | null {
  if (!details) return null;

  const candidates: (string | null | undefined)[] = [
    details.description,
    details.raw_input ? pickFromRawInput(details.raw_input) : null,
    details.command,
    details.path,
    details.cwd,
  ];

  for (const c of candidates) {
    if (!c) continue;
    const trimmed = c.trim();
    if (!trimmed) continue;
    if (trimmed === title.trim()) continue;
    return truncate(trimmed);
  }
  return null;
}
