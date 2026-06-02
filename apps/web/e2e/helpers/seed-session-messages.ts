/**
 * Build a mock-agent script that emits N messages with a small delay between
 * them. Each entry becomes one agent text event. NOTE: consecutive agent-text
 * chunks COALESCE into a single chat message — the backend only flushes a
 * distinct message row on a tool-call/turn boundary (manager_streaming.
 * flushMessageBuffer), and e2e:delay does not flush. So N lines render as one
 * growing bubble, not N rows (search specs tolerate this with N>=1 hits).
 * To seed many DISTINCT rows (e.g. for pagination tests), use the test harness
 * instead: apiClient.seedToolCallMessages / seedSessionMessage (called repeatedly
 * with type "text" for each row), which write separate rows directly. Used as the
 * `description` of a task created
 * via `apiClient.createTaskWithAgent` so the session boots with pre-seeded history.
 */
export function multiMessageScript(lines: string[], delayMs = 10): string {
  const parts: string[] = [];
  for (const line of lines) {
    const escaped = line.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
    parts.push(`e2e:message("${escaped}")`);
    if (delayMs > 0) parts.push(`e2e:delay(${delayMs})`);
  }
  return parts.join("\n");
}

/** Builder for a plan-seeding script using the create_task_plan_kandev MCP tool. */
export function planScript(content: string, title = "Search test plan"): string {
  const escape = (s: string) =>
    s.replaceAll("\\", "\\\\").replaceAll('"', '\\"').replaceAll("\n", "\\n");
  const escapedContent = escape(content);
  const escapedTitle = escape(title);
  return [
    'e2e:thinking("Seeding plan...")',
    "e2e:delay(50)",
    `e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"${escapedContent}","title":"${escapedTitle}"})`,
    "e2e:delay(50)",
    'e2e:message("Plan seeded.")',
  ].join("\n");
}
