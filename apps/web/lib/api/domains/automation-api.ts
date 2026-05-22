import { getWebSocketClient } from "@/lib/ws/connection";
import type {
  Automation,
  AutomationRun,
  CreateAutomationRequest,
  UpdateAutomationRequest,
  AddTriggerRequest,
  UpdateTriggerRequest,
  AutomationTrigger,
  TriggerTypeInfo,
} from "@/lib/types/automation";

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

export async function createAutomation(req: CreateAutomationRequest): Promise<Automation> {
  return requireClient().request<Automation>("automation.create", req);
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

export async function triggerAutomation(id: string): Promise<{ triggered: boolean }> {
  return requireClient().request<{ triggered: boolean }>("automation.trigger", { id });
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
