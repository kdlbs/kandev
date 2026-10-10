import { type TaskPlan, type TaskPlanRevision } from "@/lib/types/http";
import { notify, persist, plansByTask, respond } from "./runtime-state";

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function handlePlanRequest(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
) {
  const taskId = String(payload.task_id || "");
  const plan = plansByTask[taskId];
  if (readPlanRequest(socketId, id, action, payload)) return;
  if (action === "task.plan.delete") {
    delete plansByTask[taskId];
    persist();
    respond(socketId, id, { success: true });
    notify("task.plan.deleted", { task_id: taskId });
    return;
  }
  if (action === "task.plan.implementation_started") {
    if (!plan) {
      respond(socketId, id, { message: "Task plan not found" }, true);
      return;
    }
    const updated = {
      ...plan,
      implementation_started_at: new Date().toISOString(),
      implementation_started_session_id: String(payload.session_id || ""),
      implementation_started_by: String(payload.actor || "user"),
    };
    plansByTask[taskId] = updated;
    persist();
    respond(socketId, id, updated);
    notify("task.plan.updated", updated);
    return;
  }
  if (action === "task.plan.create" || action === "task.plan.update") {
    const updated = updatedPlan(taskId, plan, payload);
    plansByTask[taskId] = updated;
    persist();
    respond(socketId, id, updated);
    notify(action === "task.plan.create" ? "task.plan.created" : "task.plan.updated", updated);
    return;
  }
  respond(socketId, id, { message: `Unsupported plan action: ${action}` }, true);
}

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function demoPlanRevision(plan: TaskPlan, includeContent: boolean): TaskPlanRevision {
  return {
    id: `${plan.id}-revision-1`,
    task_id: plan.task_id,
    revision_number: 1,
    title: plan.title,
    ...(includeContent ? { content: plan.content } : {}),
    author_kind: plan.created_by,
    author_name: plan.created_by === "agent" ? "Mock agent" : "Demo user",
    created_at: plan.created_at,
    updated_at: plan.updated_at,
  };
}

function readPlanRequest(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  const plan = plansByTask[String(payload.task_id || "")];
  if (action === "task.plan.get") {
    respond(socketId, id, plan ?? null);
    return true;
  }
  if (action === "task.plan.revisions.list") {
    respond(socketId, id, { revisions: plan ? [demoPlanRevision(plan, false)] : [] });
    return true;
  }
  if (action === "task.plan.revision.get") {
    const revision = Object.values(plansByTask)
      .map((candidate) => demoPlanRevision(candidate, true))
      .find((candidate) => candidate.id === payload.revision_id);
    respond(socketId, id, revision ?? null);
    return true;
  }
  if (action === "task.plan.revert") {
    const revision = plan ? demoPlanRevision(plan, true) : null;
    respond(socketId, id, revision);
    return true;
  }
  return false;
}

function updatedPlan(
  taskId: string,
  plan: TaskPlan | undefined,
  payload: Record<string, unknown>,
): TaskPlan {
  const now = new Date().toISOString();
  return {
    id: plan?.id ?? `demo-plan-${taskId}`,
    task_id: taskId,
    // i18n-exempt: demo fixture content retains its seeded English text
    title: String(payload.title || plan?.title || "Plan"),
    content: String(payload.content ?? plan?.content ?? ""),
    created_by: payload.created_by === "agent" ? "agent" : "user",
    created_at: plan?.created_at ?? now,
    updated_at: now,
  };
}
