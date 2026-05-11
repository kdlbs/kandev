const KANDEV_TOOL_RE = /^[a-z][a-z0-9_]*_kandev$/;
// Different agents prefix MCP tools differently: Claude Code passes the bare
// tool name, Codex passes `<server>/<tool>`, others may use `mcp__<server>__`.
const NAMESPACE_SEP = /\/|__/;
const ACRONYMS = new Set(["mcp"]);

export function prettifyToolTitle(raw: string): string {
  if (!raw) return raw;
  const trimmed = raw.trim();
  const segments = trimmed.split(NAMESPACE_SEP);
  const tail = segments[segments.length - 1];
  if (!KANDEV_TOOL_RE.test(tail)) return trimmed;
  const stem = tail.slice(0, -"_kandev".length);
  const words = stem
    .split("_")
    .filter(Boolean)
    .map((w) => (ACRONYMS.has(w) ? w.toUpperCase() : w.charAt(0).toUpperCase() + w.slice(1)));
  return `Kandev: ${words.join(" ")}`;
}
