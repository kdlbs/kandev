import type { DemoHttpResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import type { DemoWorkflowRouteContext, DemoWorkflowRuntimeSnapshot } from "./workflow-runtime";

type Changed = (action?: string, payload?: unknown) => void;

export function routeDemoWorkflowSync(
  context: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  changed: Changed,
) {
  return routeSyncConfig(context, state, changed) ?? routeSyncNow(context, state, changed);
}

function routeSyncConfig(
  { path, method, input }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  changed: Changed,
) {
  if (path !== "/api/v1/workflow-sync/config") return null;
  if (method === "GET") return state.syncConfig ? ok(state.syncConfig) : empty();
  if (method === "DELETE") {
    state.syncConfig = null;
    changed();
    return ok({ deleted: true });
  }
  if (method !== "POST") return null;
  const owner = stringValue(input.repo_owner);
  const name = stringValue(input.repo_name);
  if (!owner || !name) return error("repo_owner and repo_name are required", 400);
  state.syncConfig = {
    workspace_id: DEMO_IDS.workspace,
    repo_owner: owner,
    repo_name: name,
    branch: stringValue(input.branch) || "main",
    path: stringValue(input.path) || ".kandev/workflows",
    interval_seconds: numberValue(input.interval_seconds, 300),
    poll_enabled: typeof input.poll_enabled === "boolean" ? input.poll_enabled : true,
    last_ok: true,
    last_warnings: [],
    created_at: state.syncConfig?.created_at ?? "2026-07-18T12:00:00.000Z",
    updated_at: new Date().toISOString(),
  };
  changed();
  return ok(state.syncConfig);
}

function routeSyncNow(
  { path, method }: DemoWorkflowRouteContext,
  state: DemoWorkflowRuntimeSnapshot,
  changed: Changed,
) {
  if (path !== "/api/v1/workflow-sync/sync" || method !== "POST") return null;
  if (!state.syncConfig) return error("workflow sync is not configured", 404);
  state.syncConfig = {
    ...state.syncConfig,
    last_synced_at: new Date().toISOString(),
    last_ok: true,
    last_error: "",
    last_warnings: [],
    updated_at: new Date().toISOString(),
  };
  changed();
  return ok({
    config: state.syncConfig,
    result: { created: [], updated: [], deleted: [], warnings: [], unchanged: true },
  });
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function ok(body: unknown, status = 200): DemoHttpResponse {
  return { status, headers: { "Content-Type": "application/json" }, body };
}

function error(message: string, status: number): DemoHttpResponse {
  return ok({ error: message }, status);
}

function empty(): DemoHttpResponse {
  return { status: 204 };
}
