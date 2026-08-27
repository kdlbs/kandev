import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentConfigBundleFile = {
  source_path: string;
  target_path: string;
  available: boolean;
};

export type AgentConfigBundle = {
  id: string;
  agent_id: string;
  display_name: string;
  label: string;
  files: AgentConfigBundleFile[];
  available: boolean;
};

export type ListAgentConfigBundlesResponse = {
  bundles: AgentConfigBundle[];
};

export async function listAgentConfigBundles(
  options?: ApiRequestOptions,
): Promise<ListAgentConfigBundlesResponse> {
  return fetchJson<ListAgentConfigBundlesResponse>("/api/v1/agent-config-bundles", {
    ...options,
    cache: "no-store",
  });
}
