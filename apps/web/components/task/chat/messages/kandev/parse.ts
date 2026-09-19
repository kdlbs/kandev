// Parsing helpers for Kandev MCP tool calls.
//
// Tool names arrive prefixed by the agent runtime:
//   - Claude Code:  `mcp__kandev__<tool>_kandev`
//   - Codex:        `kandev/<tool>_kandev`
//   - Bare:         `<tool>_kandev`
//
// `extractKandevStem` strips the namespace and the trailing `_kandev` suffix so
// the dispatcher can match on the short stem (`list_tasks`, `create_task`, …).
//
// `extractMcpResult` unwraps the MCP CallToolResult shape that lands in
// `metadata.normalized.generic.output` (or `metadata.result`). The content is
// usually an array of `{type, text}` content blocks where `text` is itself a
// JSON-encoded string; we parse that inner JSON so the renderers see plain JS.

const NAMESPACE_SEP = /\/|__|\./;
const KANDEV_NAMESPACES = ["mcp__kandev__", "mcp.kandev.", "kandev/"] as const;
const KANDEV_SUFFIX = "_kandev";

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

// ACP stores generic tool arguments below `input.raw_input`; older/direct
// transports store the arguments at `input`. Normalize both at the renderer
// boundary so every Kandev tool sees its actual call arguments.
export function extractKandevArgs(value: unknown): Record<string, unknown> | undefined {
  const input = asRecord(value);
  if (!input) return undefined;
  const rawInput = asRecord(input.raw_input);
  return asRecord(rawInput?.arguments) ?? rawInput ?? input;
}

export function extractKandevStem(toolName: string | undefined): string | null {
  if (!toolName) return null;
  const normalized = toolName.trim();
  const namespace = KANDEV_NAMESPACES.find((prefix) => normalized.startsWith(prefix));
  const tail = namespace ? normalized.slice(namespace.length) : normalized;
  if ((!namespace && NAMESPACE_SEP.test(normalized)) || NAMESPACE_SEP.test(tail)) return null;
  if (!tail.endsWith(KANDEV_SUFFIX)) return null;
  const stem = tail.slice(0, -KANDEV_SUFFIX.length);
  return stem.length > 0 ? stem : null;
}

export function isKandevTool(toolName: string | undefined): boolean {
  return extractKandevStem(toolName) !== null;
}

type ContentBlock = { type?: string; text?: string };

function tryParseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function unwrapContentBlocks(blocks: unknown[]): unknown {
  const textParts: string[] = [];
  for (const block of blocks) {
    if (!block || typeof block !== "object") continue;
    const b = block as ContentBlock;
    if (typeof b.text === "string") textParts.push(b.text);
  }
  if (textParts.length === 0) return blocks;
  const joined = textParts.join("");
  return tryParseJson(joined);
}

function hasValue(value: unknown): boolean {
  return value !== undefined && value !== null;
}

// extractMcpResult walks the various shapes the ACP/MCP transport can emit and
// returns the structured payload. Returns null if the value is empty/missing.
export function extractMcpResult(value: unknown): unknown {
  if (value === null || value === undefined) return null;
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) return null;
    return tryParseJson(trimmed);
  }
  if (Array.isArray(value)) {
    return unwrapContentBlocks(value);
  }
  if (typeof value === "object") {
    const obj = value as {
      content?: unknown;
      output?: unknown;
      result?: unknown;
      structuredContent?: unknown;
      structured_content?: unknown;
    };
    if (hasValue(obj.structuredContent)) {
      return obj.structuredContent;
    }
    if (hasValue(obj.structured_content)) {
      return obj.structured_content;
    }
    if (Array.isArray(obj.content)) return unwrapContentBlocks(obj.content);
    if (typeof obj.output === "string") return tryParseJson(obj.output);
    if (
      obj.result !== undefined &&
      (Object.hasOwn(obj, "error") || Object.keys(obj).length === 1)
    ) {
      return extractMcpResult(obj.result);
    }
    return value;
  }
  return value;
}

export function pickString(obj: unknown, key: string): string | undefined {
  if (!obj || typeof obj !== "object") return undefined;
  const v = (obj as Record<string, unknown>)[key];
  return typeof v === "string" ? v : undefined;
}

export function pickNumber(obj: unknown, key: string): number | undefined {
  if (!obj || typeof obj !== "object") return undefined;
  const v = (obj as Record<string, unknown>)[key];
  return typeof v === "number" ? v : undefined;
}

export function pickArray<T = unknown>(obj: unknown, key: string): T[] | undefined {
  if (!obj || typeof obj !== "object") return undefined;
  const v = (obj as Record<string, unknown>)[key];
  return Array.isArray(v) ? (v as T[]) : undefined;
}

export function pickObject(obj: unknown, key: string): Record<string, unknown> | undefined {
  const record = asRecord(obj);
  if (!record) return undefined;
  return asRecord(record[key]);
}

// Shorten a UUID-like identifier for inline display (e.g. "abc12345…").
const SHORT_ID_LEN = 8;
export function shortId(id: string | undefined): string {
  if (!id) return "";
  if (id.length <= SHORT_ID_LEN + 1) return id;
  return `${id.slice(0, SHORT_ID_LEN)}…`;
}
