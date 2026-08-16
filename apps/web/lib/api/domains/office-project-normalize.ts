import type { Project } from "@/lib/state/slices/office/types";

function rawField(record: Record<string, unknown>, camelKey: string, snakeKey: string) {
  return record[camelKey] ?? record[snakeKey];
}

function stringField(record: Record<string, unknown>, camelKey: string, snakeKey: string) {
  const value = rawField(record, camelKey, snakeKey);
  return typeof value === "string" ? value : "";
}

function numberField(
  record: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
  fallback: number,
) {
  const value = rawField(record, camelKey, snakeKey);
  return typeof value === "number" ? value : fallback;
}

function parseJSONField<T>(value: unknown, fallback: T): T {
  if (typeof value !== "string") return (value as T) ?? fallback;
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

/** Converts the backend snake_case project wire shape to the web model. */
export function normalizeProject(raw: unknown): Project {
  const project =
    raw && typeof raw === "object"
      ? (raw as Record<string, unknown>)
      : ({} as Record<string, unknown>);
  const taskCounts = rawField(project, "taskCounts", "task_counts");
  const status = rawField(project, "status", "status");
  return {
    id: stringField(project, "id", "id"),
    workspaceId: stringField(project, "workspaceId", "workspace_id"),
    name: stringField(project, "name", "name"),
    description: stringField(project, "description", "description"),
    status: (typeof status === "string" ? status : "active") as Project["status"],
    leadAgentProfileId: stringField(project, "leadAgentProfileId", "lead_agent_profile_id"),
    color: stringField(project, "color", "color"),
    budgetCents: numberField(project, "budgetCents", "budget_cents", 0),
    repositories: parseJSONField<string[]>(rawField(project, "repositories", "repositories"), []),
    executorConfig: parseJSONField<Record<string, unknown>>(
      rawField(project, "executorConfig", "executor_config"),
      {},
    ),
    taskCounts:
      taskCounts && typeof taskCounts === "object"
        ? (taskCounts as Project["taskCounts"])
        : undefined,
    createdAt: stringField(project, "createdAt", "created_at"),
    updatedAt: stringField(project, "updatedAt", "updated_at"),
  };
}
