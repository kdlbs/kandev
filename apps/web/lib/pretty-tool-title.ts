const KANDEV_TOOL_RE = /^[a-z][a-z0-9_]*_kandev$/;
const ACRONYMS = new Set(["mcp", "id", "url", "api", "ui", "vs", "ide", "cli"]);

export function prettifyToolTitle(raw: string): string {
  if (!raw) return raw;
  const trimmed = raw.trim();
  if (!KANDEV_TOOL_RE.test(trimmed)) return raw;
  const stem = trimmed.slice(0, -"_kandev".length);
  const words = stem
    .split("_")
    .filter(Boolean)
    .map((w) => (ACRONYMS.has(w) ? w.toUpperCase() : w.charAt(0).toUpperCase() + w.slice(1)));
  return `Kandev: ${words.join(" ")}`;
}
