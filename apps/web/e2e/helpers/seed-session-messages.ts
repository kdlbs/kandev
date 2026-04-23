/**
 * Build a mock-agent script that emits N messages with a small delay between
 * them. Each entry becomes one agent text event, producing one distinct message
 * row in the chat panel. Used as the `description` of a task created via
 * `apiClient.createTaskWithAgent` so the session boots with pre-seeded history.
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
  const escaped = content.replaceAll("\\", "\\\\").replaceAll('"', '\\"').replaceAll("\n", "\\n");
  return [
    'e2e:thinking("Seeding plan...")',
    "e2e:delay(50)",
    `e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"${escaped}","title":"${title}"})`,
    "e2e:delay(50)",
    'e2e:message("Plan seeded.")',
  ].join("\n");
}
