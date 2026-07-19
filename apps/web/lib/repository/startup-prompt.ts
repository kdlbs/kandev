/**
 * Client-side mirror of the backend's scriptengine.ResolveStartupPrompt.
 *
 * The manual Create Task dialog only ever has {{TASK_TITLE}} available — ticket
 * placeholders (TICKET_ID, TICKET_URL, TICKET_PROVIDER) require metadata that
 * only lands on watcher-imported or parent-inherited tasks, so any line
 * referencing them is dropped on the client. Server-side resolution still
 * runs on submit for the authoritative substitution.
 */
// PLACEHOLDER_TOKEN captures every `{{KEY}}` in a line so ResolveStartupPrompt
// can look up KEY against the known-vars set. Checking against the ORIGINAL
// line's tokens (not the resolved string) avoids a false drop when the
// substituted task title itself contains a `{{...}}` literal.
const PLACEHOLDER_TOKEN = /\{\{([^}]*)\}\}/g;

// Client-side the manual Create Task dialog only knows TASK_TITLE. Any other
// placeholder in the prompt (TICKET_ID, TICKET_URL, TICKET_PROVIDER) is
// unresolved — its line gets dropped from the pre-fill until server-side
// resolution can supply the ticket metadata on submit.
const KNOWN_KEYS = new Set(["TASK_TITLE"]);

/**
 * Resolves `{{TASK_TITLE}}` and drops any line whose ticket placeholders
 * never resolved. Leading/trailing newlines are trimmed.
 */
export function resolveStartupPromptForManualDialog(prompt: string, taskTitle: string): string {
  if (!prompt) return "";
  // Normalize CRLF to LF so prompts saved by Windows editors don't leave a
  // stray \r in each resolved line.
  const lines = prompt.replace(/\r\n/g, "\n").split("\n");
  const kept: string[] = [];
  for (const line of lines) {
    if (hasUnknownPlaceholder(line)) continue;
    // Callback form of replace so `$&`, `$$`, `$'`, `$\`` in the task title
    // are inserted literally instead of being interpreted as substitution
    // patterns.
    kept.push(line.replace(/\{\{TASK_TITLE\}\}/g, () => taskTitle));
  }
  // Trim only newlines — preserve any leading/trailing spaces or tabs a
  // user intentionally put on the first or last kept line (e.g. indented
  // bullet content). Collapse to "" when the whole result is blank so the
  // dialog's hasDescription state matches what the user sees.
  const trimmed = kept.join("\n").replace(/^\n+|\n+$/g, "");
  return trimmed.trim() === "" ? "" : trimmed;
}

function hasUnknownPlaceholder(line: string): boolean {
  for (const match of line.matchAll(PLACEHOLDER_TOKEN)) {
    if (!KNOWN_KEYS.has(match[1])) return true;
  }
  return false;
}
