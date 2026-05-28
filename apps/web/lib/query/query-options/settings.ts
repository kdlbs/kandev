import { queryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import {
  listExecutors,
  listAgents,
  listAgentDiscovery,
  listAvailableAgents,
  listEditors,
  listPrompts,
  listNotificationProviders,
  fetchUserSettings,
  listInstallJobs,
  listRemoteCredentials,
} from "@/lib/api/domains/settings-api";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import { getSpritesStatus, listSpritesInstances } from "@/lib/api/domains/sprites-api";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent } from "@/lib/types/http";

function normalizeAgentResponse(agent: Agent): Agent {
  return {
    ...agent,
    profiles: (agent.profiles ?? []).map((profile) => normalizeAgentProfile(profile)),
  };
}

export const settingsQueryOptions = {
  executors: () =>
    queryOptions({
      queryKey: qk.settings.executors(),
      queryFn: () => listExecutors({ cache: "no-store" }).then((r) => r.executors ?? []),
    }),

  agents: () =>
    queryOptions({
      queryKey: qk.settings.agents(),
      queryFn: async () => {
        const r = await listAgents({ cache: "no-store" });
        return (r.agents ?? []).map(normalizeAgentResponse);
      },
    }),

  agentProfiles: () =>
    queryOptions({
      queryKey: qk.settings.agentProfiles(),
      queryFn: async () => {
        const r = await listAgents({ cache: "no-store" });
        const agents = (r.agents ?? []).map(normalizeAgentResponse);
        return agents.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        );
      },
    }),

  agentDiscovery: () =>
    queryOptions({
      queryKey: qk.settings.agentDiscovery(),
      queryFn: () => listAgentDiscovery({ cache: "no-store" }).then((r) => r.agents ?? []),
      staleTime: 5 * 60_000,
    }),

  availableAgents: () =>
    queryOptions({
      queryKey: qk.settings.availableAgents(),
      queryFn: async () => {
        const r = await listAvailableAgents({ cache: "no-store" });
        return { agents: r.agents ?? [], tools: r.tools ?? [] };
      },
    }),

  editors: () =>
    queryOptions({
      queryKey: qk.settings.editors(),
      queryFn: () => listEditors({ cache: "no-store" }).then((r) => r.editors ?? []),
    }),

  prompts: () =>
    queryOptions({
      queryKey: qk.settings.prompts(),
      queryFn: () => listPrompts({ cache: "no-store" }).then((r) => r.prompts ?? []),
    }),

  secrets: () =>
    queryOptions({
      queryKey: qk.settings.secrets(),
      queryFn: () => listSecrets({ cache: "no-store" }),
    }),

  sprites: (secretId?: string) =>
    queryOptions({
      queryKey: qk.settings.sprites(secretId),
      queryFn: async () => {
        const [status, instances] = await Promise.all([
          getSpritesStatus(secretId, { cache: "no-store" }),
          listSpritesInstances(secretId, { cache: "no-store" }),
        ]);
        return { status, instances: instances ?? [] };
      },
      enabled: secretId !== undefined,
    }),

  notificationProviders: () =>
    queryOptions({
      queryKey: qk.settings.notificationProviders(),
      queryFn: async () => {
        const r = await listNotificationProviders({ cache: "no-store" });
        return {
          items: r.providers ?? [],
          events: r.events ?? [],
          appriseAvailable: r.apprise_available ?? false,
        };
      },
    }),

  userSettings: () =>
    queryOptions({
      queryKey: qk.settings.userSettings(),
      queryFn: () => fetchUserSettings({ cache: "no-store" }),
      staleTime: 5 * 60_000,
    }),

  installJobs: () =>
    queryOptions({
      queryKey: qk.settings.installJobs(),
      queryFn: () => listInstallJobs({ cache: "no-store" }).then((r) => r.jobs ?? []),
    }),

  systemHealth: () =>
    queryOptions({
      queryKey: qk.settings.systemHealth(),
      queryFn: () => fetchSystemHealth({ cache: "no-store" }),
    }),

  remoteAuthSpecs: () =>
    queryOptions({
      queryKey: qk.settings.remoteAuthSpecs(),
      queryFn: () => listRemoteCredentials({ cache: "no-store" }).then((r) => r.auth_specs ?? []),
      staleTime: Infinity,
    }),
};
