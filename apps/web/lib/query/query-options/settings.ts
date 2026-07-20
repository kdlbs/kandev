import { queryOptions } from "@tanstack/react-query";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import { fetchRuntimeFlags } from "@/lib/api/domains/runtime-flags-api";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import { getSpritesStatus, listSpritesInstances } from "@/lib/api/domains/sprites-api";
import {
  fetchDefaultScripts,
  fetchDynamicModels,
  fetchExecutor,
  fetchSystemMetricsSettings,
  fetchUserSettings,
  getAgentProfileMcpConfig,
  getInstallJob,
  listAgentDiscovery,
  listAgents,
  listAllExecutorProfiles,
  listAvailableAgents,
  listEditors,
  listExecutorProfiles,
  listExecutors,
  listInstallJobs,
  listNotificationProviders,
  listAgentSubscriptionUsage,
  listPrompts,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function userSettingsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.user(),
    queryFn: ({ signal }) => fetchUserSettings(withSignal(signal)),
  });
}

export function systemMetricsSettingsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.systemMetrics(),
    queryFn: ({ signal }) => fetchSystemMetricsSettings(withSignal(signal)),
  });
}

export function executorsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.executors(),
    queryFn: ({ signal }) => listExecutors(withSignal(signal)),
  });
}

export function executorQueryOptions(executorId: string) {
  return queryOptions({
    queryKey: qk.settings.executor(executorId),
    queryFn: ({ signal }) => fetchExecutor(executorId, withSignal(signal)),
    enabled: Boolean(executorId),
  });
}

export function executorProfilesQueryOptions(executorId: string) {
  return queryOptions({
    queryKey: qk.settings.executorProfiles(executorId),
    queryFn: ({ signal }) => listExecutorProfiles(executorId, withSignal(signal)),
    enabled: Boolean(executorId),
  });
}

export function allExecutorProfilesQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.allExecutorProfiles(),
    queryFn: ({ signal }) => listAllExecutorProfiles(withSignal(signal)),
  });
}

export function scriptPlaceholdersQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.scriptPlaceholders(),
    queryFn: ({ signal }) => listScriptPlaceholders(withSignal(signal)),
  });
}

export function defaultScriptsQueryOptions(executorType: string) {
  return queryOptions({
    queryKey: qk.settings.defaultScripts(executorType),
    queryFn: ({ signal }) => fetchDefaultScripts(executorType, withSignal(signal)),
    enabled: Boolean(executorType),
  });
}

export function agentsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.agents(),
    queryFn: ({ signal }) => listAgents(withSignal(signal)),
  });
}

export function agentDiscoveryQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.agentDiscovery(),
    queryFn: ({ signal }) => listAgentDiscovery(withSignal(signal)),
  });
}

export function availableAgentsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.availableAgents(),
    queryFn: ({ signal }) => listAvailableAgents(withSignal(signal)),
  });
}

export function agentSubscriptionUsageQueryOptions(fresh = false) {
  return queryOptions({
    queryKey: qk.settings.agentSubscriptionUsage(),
    queryFn: ({ signal }) =>
      listAgentSubscriptionUsage({ ...withSignal(signal), cache: "no-store", fresh }),
  });
}

export function agentProfileMcpConfigQueryOptions(profileId: string) {
  return queryOptions({
    queryKey: qk.settings.agentMcpConfig(profileId),
    queryFn: ({ signal }) => getAgentProfileMcpConfig(profileId, withSignal(signal)),
    enabled: Boolean(profileId),
  });
}

export function installJobsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.installJobs(),
    queryFn: ({ signal }) => listInstallJobs(withSignal(signal)),
  });
}

export function installJobQueryOptions(jobId: string) {
  return queryOptions({
    queryKey: qk.settings.installJob(jobId),
    queryFn: ({ signal }) => getInstallJob(jobId, withSignal(signal)),
    enabled: Boolean(jobId),
  });
}

export function dynamicModelsQueryOptions(agentName: string, params?: { refresh?: boolean }) {
  return queryOptions({
    queryKey: qk.settings.dynamicModels(agentName),
    queryFn: ({ signal }) =>
      fetchDynamicModels(agentName, { ...withSignal(signal), refresh: params?.refresh }),
    enabled: Boolean(agentName),
  });
}

export function editorsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.editors(),
    queryFn: ({ signal }) => listEditors(withSignal(signal)),
  });
}

export function promptsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.prompts(),
    queryFn: ({ signal }) => listPrompts(withSignal(signal)),
  });
}

export function notificationProvidersQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.notificationProviders(),
    queryFn: ({ signal }) => listNotificationProviders(withSignal(signal)),
  });
}

export function secretsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.secrets(),
    queryFn: ({ signal }) => listSecrets(withSignal(signal)),
  });
}

export function spritesStatusQueryOptions(secretId?: string | null) {
  return queryOptions({
    queryKey: qk.settings.spritesStatus(secretId),
    queryFn: ({ signal }) => getSpritesStatus(secretId ?? undefined, withSignal(signal)),
    enabled: secretId !== null,
  });
}

export function spritesInstancesQueryOptions(secretId?: string | null) {
  return queryOptions({
    queryKey: qk.settings.spritesInstances(secretId),
    queryFn: ({ signal }) => listSpritesInstances(secretId ?? undefined, withSignal(signal)),
    enabled: secretId !== null,
  });
}

export function systemHealthQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.systemHealth(),
    queryFn: ({ signal }) => fetchSystemHealth(withSignal(signal)),
    refetchInterval: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
  });
}

export function runtimeFlagsQueryOptions() {
  return queryOptions({
    queryKey: qk.settings.runtimeFlags(),
    queryFn: () => fetchRuntimeFlags(),
    staleTime: 30_000,
  });
}
