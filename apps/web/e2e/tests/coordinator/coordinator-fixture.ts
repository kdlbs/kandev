import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";

export class CoordinatorFixtureUnavailable extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CoordinatorFixtureUnavailable";
  }
}

export type CoordinatorRecord = {
  id: string;
  workspace_id: string;
  name: string;
  agent_profile_id: string;
  executor_profile_id: string;
  context: string;
  conversation_task_id: string | null;
  created_at: string;
  updated_at: string;
  open_proposals?: number;
  agent_profile_status?: "ok" | "missing" | "passthrough";
  executor_profile_status?: "ok" | "missing";
};

export async function readCoordinatorFeature(apiClient: ApiClient): Promise<boolean | null> {
  const response = await apiClient.rawRequest("GET", "/api/v1/features");
  if (!response.ok) return null;
  const body = (await response.json()) as { coordinator?: unknown };
  return typeof body.coordinator === "boolean" ? body.coordinator : null;
}

export async function enableCoordinatorFeature(
  backend: BackendContext,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<() => Promise<void>> {
  const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR: "true" });
  const enabled = await readCoordinatorFeature(apiClient);
  if (enabled !== true) {
    await release();
    throw new CoordinatorFixtureUnavailable(
      "Coordinator fixture skipped: KANDEV_FEATURES_COORDINATOR could not be enabled.",
    );
  }

  const probe = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinators`,
  );
  if (!probe.ok) {
    await release();
    throw new CoordinatorFixtureUnavailable(
      `Coordinator fixture skipped: coordinator API is unavailable (${probe.status}).`,
    );
  }
  return release;
}

export async function seedCoordinator(
  apiClient: ApiClient,
  workspaceId: string,
  opts: {
    name: string;
    agentProfileId: string;
    executorProfileId: string;
    context?: string;
  },
): Promise<CoordinatorRecord> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinators`,
    {
      name: opts.name,
      agent_profile_id: opts.agentProfileId,
      executor_profile_id: opts.executorProfileId,
      context: opts.context ?? "",
    },
  );
  if (!response.ok) {
    throw new Error(`Coordinator seeding failed (${response.status}): ${await response.text()}`);
  }
  return (await response.json()) as CoordinatorRecord;
}

export async function deleteCoordinator(
  apiClient: ApiClient,
  workspaceId: string,
  coordinatorId: string,
): Promise<void> {
  await apiClient.rawRequest(
    "DELETE",
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinators/${encodeURIComponent(coordinatorId)}`,
  );
}
