import { getWebSocketClient } from "@/lib/ws/connection";
import type {
  Automation,
  AutomationRun,
  AutomationSummary,
  CreateAutomationRequest,
  CreateAutomationResponse,
  RevealWebhookSecretResponse,
  UpdateAutomationRequest,
  AddTriggerRequest,
  UpdateTriggerRequest,
  AutomationTrigger,
  TriggerTypeInfo,
  WorkspaceAutomationRun,
} from "@/lib/types/automation";

// i18n-exempt: precondition diagnostic for a programmer error; callers branch
// on the error type, never render this message.
const WS_UNAVAILABLE = "WebSocket client not available";

function requireClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_UNAVAILABLE);
  return client;
}

export async function listAutomations(workspaceId: string): Promise<Automation[]> {
  return requireClient().request<Automation[]>("automation.list", { workspace_id: workspaceId });
}

export async function getAutomation(id: string): Promise<Automation> {
  return requireClient().request<Automation>("automation.get", { id });
}

export async function createAutomation(
  req: CreateAutomationRequest,
): Promise<CreateAutomationResponse> {
  return requireClient().request<CreateAutomationResponse>("automation.create", req);
}

export async function revealWebhookSecret(
  automationId: string,
  workspaceId: string,
): Promise<RevealWebhookSecretResponse> {
  return requireClient().request<RevealWebhookSecretResponse>("automation.webhook.reveal_secret", {
    id: automationId,
    workspace_id: workspaceId,
  });
}

export async function updateAutomation(
  id: string,
  req: UpdateAutomationRequest,
): Promise<Automation> {
  return requireClient().request<Automation>("automation.update", { id, ...req });
}

export async function deleteAutomation(id: string): Promise<void> {
  await requireClient().request("automation.delete", { id });
}

export async function enableAutomation(id: string): Promise<Automation> {
  return requireClient().request<Automation>("automation.enable", { id });
}

export async function disableAutomation(id: string): Promise<Automation> {
  return requireClient().request<Automation>("automation.disable", { id });
}

/**
 * Fire an automation by hand. A request can succeed without anything running —
 * the concurrency cap turns a trigger away while an earlier run is still in
 * flight — so callers must read `skipped` rather than assume a fire.
 */
export type TriggerAutomationResult = {
  triggered: boolean;
  skipped?: boolean;
  reason?: string;
};

export async function triggerAutomation(id: string): Promise<TriggerAutomationResult> {
  return requireClient().request<TriggerAutomationResult>("automation.trigger", { id });
}

export async function listAutomationRuns(
  automationId: string,
  limit?: number,
): Promise<AutomationRun[]> {
  return requireClient().request<AutomationRun[]>("automation.runs.list", {
    automation_id: automationId,
    ...(limit ? { limit } : {}),
  });
}

/**
 * Every automation's runs in one feed, newest first. Unlike the per-automation
 * log this one is envelope-shaped (`{runs}`) — tolerate a missing list so a
 * workspace that has never fired anything renders the empty state instead of
 * throwing.
 */
export async function listWorkspaceAutomationRuns(
  workspaceId: string,
  limit?: number,
): Promise<WorkspaceAutomationRun[]> {
  const response = await requireClient().request<{ runs: WorkspaceAutomationRun[] }>(
    "automation.runs.list_workspace",
    { workspace_id: workspaceId, ...(limit ? { limit } : {}) },
  );
  return response?.runs ?? [];
}

/**
 * One health summary per automation in the workspace that has ever run.
 * Envelope-shaped for the same reason the feed is; an automation with no row
 * has never run, which the caller reads as "no runs yet".
 */
export async function listAutomationSummaries(workspaceId: string): Promise<AutomationSummary[]> {
  const response = await requireClient().request<{ summaries: AutomationSummary[] }>(
    "automation.summaries",
    { workspace_id: workspaceId },
  );
  return response?.summaries ?? [];
}

/**
 * One automation's summary, or null when it has never run. The detail page
 * reads this rather than counting open runs in its own history window: that
 * window is capped, so an open run older than it would leave the page claiming
 * nothing is in flight.
 */
export async function getAutomationSummary(
  automationId: string,
): Promise<AutomationSummary | null> {
  const response = await requireClient().request<{ summary: AutomationSummary | null }>(
    "automation.summary",
    { automation_id: automationId },
  );
  return response?.summary ?? null;
}

export async function addTrigger(req: AddTriggerRequest): Promise<AutomationTrigger> {
  return requireClient().request<AutomationTrigger>("automation.trigger.add", req);
}

export async function updateTrigger(
  id: string,
  req: UpdateTriggerRequest,
): Promise<{ updated: boolean }> {
  return requireClient().request<{ updated: boolean }>("automation.trigger.update", { id, ...req });
}

export async function deleteTrigger(id: string): Promise<{ deleted: boolean }> {
  return requireClient().request<{ deleted: boolean }>("automation.trigger.delete", { id });
}

export async function listTriggerTypes(): Promise<TriggerTypeInfo[]> {
  return requireClient().request<TriggerTypeInfo[]>("automation.trigger_types", {});
}

export async function deleteAutomationRun(
  runId: string,
  workspaceId: string,
): Promise<{ deleted: boolean }> {
  return requireClient().request<{ deleted: boolean }>("automation.run.delete", {
    run_id: runId,
    workspace_id: workspaceId,
  });
}

export async function deleteAllAutomationRuns(
  automationId: string,
  workspaceId: string,
): Promise<{ deleted: boolean }> {
  return requireClient().request<{ deleted: boolean }>("automation.runs.delete_all", {
    automation_id: automationId,
    workspace_id: workspaceId,
  });
}

export async function stopAutomationRun(
  automationId: string,
  runId: string,
): Promise<{ run_id: string; status: string }> {
  return requireClient().request<{ run_id: string; status: string }>("automation.run.stop", {
    automation_id: automationId,
    run_id: runId,
  });
}
