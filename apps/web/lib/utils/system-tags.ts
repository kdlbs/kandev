/**
 * Strip internal `<kandev-system>...</kandev-system>` blocks from a message
 * body. These wrap context the backend injects but never wants surfaced to the
 * user (e.g. ambient task IDs, MCP session info). Used by the chat message
 * renderer and the queued-message preview so both consistently hide them.
 */
export function stripSystemTags(text: string): string {
  return text.replace(/<kandev-system>[\s\S]*?<\/kandev-system>/g, "").trim();
}
