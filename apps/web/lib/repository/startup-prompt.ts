/**
 * Client-side mirror of the backend's scriptengine.ResolveStartupPrompt.
 *
 * The manual Create Task dialog only ever has {{TASK_TITLE}} available — ticket
 * placeholders (TICKET_ID, TICKET_URL, TICKET_PROVIDER) require metadata that
 * only lands on watcher-imported or parent-inherited tasks, so any line
 * referencing them is dropped on the client. Server-side resolution still
 * runs on submit for the authoritative substitution.
 */
const UNRESOLVED_PLACEHOLDER = /\{\{[^}]*\}\}/;

/**
 * Resolves `{{TASK_TITLE}}` and drops any line whose ticket placeholders
 * never resolved. Leading/trailing whitespace is trimmed.
 */
export function resolveStartupPromptForManualDialog(prompt: string, taskTitle: string): string {
  if (!prompt) return "";
  // Normalize CRLF to LF so prompts saved by Windows editors don't leave a
  // stray \r in each resolved line.
  const lines = prompt.replace(/\r\n/g, "\n").split("\n");
  const kept: string[] = [];
  for (const line of lines) {
    const resolved = line.replace(/\{\{TASK_TITLE\}\}/g, taskTitle);
    if (UNRESOLVED_PLACEHOLDER.test(resolved)) continue;
    kept.push(resolved);
  }
  return kept.join("\n").replace(/^[\s\n]+|[\s\n]+$/g, "");
}
