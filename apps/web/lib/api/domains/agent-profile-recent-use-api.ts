import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import type {
  AgentProfileRecentUseApiRecord,
  AgentProfileRecentUseContext,
} from "@/lib/types/http-agent-profile-recent-use";

const AGENT_PROFILE_RECENT_USE_PATH = "/api/v1/user/agent-profile-recent-use";

export async function fetchAgentProfileRecentUse(options?: ApiRequestOptions) {
  return fetchJsonWithRetry<AgentProfileRecentUseApiRecord[]>(
    AGENT_PROFILE_RECENT_USE_PATH,
    options,
  );
}

export async function recordAgentProfileRecentUse(
  context: AgentProfileRecentUseContext,
  agentProfileId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<AgentProfileRecentUseApiRecord>(
    `${AGENT_PROFILE_RECENT_USE_PATH}/${encodeURIComponent(context)}`,
    {
      ...options,
      init: {
        method: "PUT",
        body: JSON.stringify({ agent_profile_id: agentProfileId }),
        ...(options?.init ?? {}),
      },
    },
  );
}
