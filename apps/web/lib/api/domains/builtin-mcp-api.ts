import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  BuiltinMCPChoices,
  BuiltinMCPManagementView,
  BuiltinMCPPolicy,
  BuiltinMCPPolicyChoice,
  BuiltinMCPPreference,
  BuiltinMCPSessionView,
  BuiltinMCPSnapshot,
} from "@/lib/types/builtin-mcp";

export function getBuiltinMCPManagement(
  scope: "workspaces" | "tasks",
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<BuiltinMCPManagementView>(
    `/api/v1/${scope}/${encodeURIComponent(id)}/mcp-built-in`,
    { ...options, cache: "no-store" },
  );
}

export function getBuiltinMCPSession(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<BuiltinMCPSessionView>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/mcp-built-in`,
    { ...options, cache: "no-store" },
  );
}

export function saveBuiltinMCPPolicy(
  workspaceId: string,
  revision: number,
  packages: BuiltinMCPChoices<BuiltinMCPPolicyChoice>,
) {
  return fetchJson<BuiltinMCPPolicy>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/mcp-built-in/policy`,
    {
      init: {
        method: "PUT",
        body: JSON.stringify({ version: 1, expected_revision: revision, packages }),
      },
    },
  );
}

export function saveBuiltinMCPPreferences(
  taskId: string,
  revision: number,
  policyRevision: number,
  packages: BuiltinMCPChoices<BuiltinMCPPreference>,
) {
  return fetchJson<BuiltinMCPSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/mcp-built-in/preferences`,
    {
      init: {
        method: "PUT",
        body: JSON.stringify({
          version: 1,
          expected_revision: revision,
          expected_workspace_policy_revision: policyRevision,
          packages,
        }),
      },
    },
  );
}
