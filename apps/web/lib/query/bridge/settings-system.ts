import type { QueryClient } from "@tanstack/react-query";
import type {
  AgentInstallJobPayload,
  BackendMessageMap,
  ExecutorProfilePayload,
} from "@/lib/types/backend";
import type { InstallJob } from "@/lib/api/domains/settings-api";
import type {
  GitHubRateLimitInfo,
  GitHubRateLimitUpdate,
  TaskCIAutomationOptions,
  TaskPR,
} from "@/lib/types/github";
import type { ListAvailableAgentsResponse } from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { SystemJob } from "@/lib/types/system";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "../keys";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

export function registerSettingsSystemBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "agent.available.updated": (message) => {
      queryClient.setQueryData<ListAvailableAgentsResponse>(qk.settings.availableAgents(), {
        agents: message.payload.agents ?? [],
        tools: message.payload.tools ?? [],
        total: message.payload.agents?.length ?? 0,
      });
    },
    "agent.install.started": (message) => patchInstallJob(queryClient, message.payload),
    "agent.install.output": (message) => patchInstallOutput(queryClient, message.payload),
    "agent.install.finished": (message) => patchInstallJob(queryClient, message.payload),
    "agent.updated": () => invalidateAgentSettings(queryClient),
    "agent.profile.created": () => invalidateAgentSettings(queryClient),
    "agent.profile.updated": () => invalidateAgentSettings(queryClient),
    "agent.profile.deleted": () => invalidateAgentSettings(queryClient),
    "executor.created": () => invalidateExecutorSettings(queryClient),
    "executor.updated": () => invalidateExecutorSettings(queryClient),
    "executor.deleted": () => invalidateExecutorSettings(queryClient),
    "executor.profile.created": (message) => patchExecutorProfile(queryClient, message.payload),
    "executor.profile.updated": (message) => patchExecutorProfile(queryClient, message.payload),
    "executor.profile.deleted": (message) => {
      removeExecutorProfile(queryClient, message.payload.id);
      invalidateExecutorSettings(queryClient);
    },
    "environment.created": () => invalidateExecutorSettings(queryClient),
    "environment.updated": () => invalidateExecutorSettings(queryClient),
    "environment.deleted": () => invalidateExecutorSettings(queryClient),
    "github.task_pr.updated": (message) => patchTaskPr(queryClient, message.payload),
    "github.task_ci_options.updated": (message) => patchTaskCiOptions(queryClient, message.payload),
    "github.rate_limit.updated": (message) => {
      queryClient.setQueryData(qk.integrations.github.rateLimit(), message.payload);
      patchGitHubStatusRateLimit(queryClient, message.payload);
    },
    "secrets.created": (message) => upsertSecret(queryClient, message.payload),
    "secrets.updated": (message) => upsertSecret(queryClient, message.payload),
    "secrets.deleted": (message) => {
      queryClient.setQueryData<SecretListItem[]>(qk.settings.secrets(), (prev) =>
        (prev ?? []).filter((secret) => secret.id !== message.payload.id),
      );
    },
    "system.job.update": (message) => patchSystemJob(queryClient, message.payload),
    "system.metrics.updated": (message) => {
      queryClient.setQueryData(qk.system.metrics(), message.payload);
    },
    "user.settings.updated": () => {
      queryClient.invalidateQueries({ queryKey: qk.settings.user() });
    },
  });
}

function invalidateAgentSettings(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: qk.settings.agents() });
  queryClient.invalidateQueries({ queryKey: qk.settings.agentDiscovery() });
  queryClient.invalidateQueries({ queryKey: qk.settings.availableAgents() });
}

function invalidateExecutorSettings(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: qk.settings.executors() });
  queryClient.invalidateQueries({ queryKey: qk.settings.allExecutorProfiles() });
}

function patchInstallJob(queryClient: QueryClient, job: AgentInstallJobPayload) {
  queryClient.setQueryData(qk.settings.installJob(job.job_id), job);
  queryClient.setQueryData<{ jobs: InstallJob[] }>(qk.settings.installJobs(), (prev) => ({
    jobs: upsertBy(prev?.jobs ?? [], job, (item) => item.job_id),
  }));
}

function patchInstallOutput(
  queryClient: QueryClient,
  payload: BackendMessageMap["agent.install.output"]["payload"],
) {
  const patch = (job: InstallJob | undefined): InstallJob | undefined =>
    job ? { ...job, output: `${job.output ?? ""}${payload.chunk}` } : job;
  queryClient.setQueryData<InstallJob>(qk.settings.installJob(payload.job_id), patch);
  queryClient.setQueryData<{ jobs: InstallJob[] }>(qk.settings.installJobs(), (prev) => ({
    jobs: (prev?.jobs ?? []).map((job) => (job.job_id === payload.job_id ? patch(job)! : job)),
  }));
}

function patchExecutorProfile(queryClient: QueryClient, payload: ExecutorProfilePayload) {
  queryClient.invalidateQueries({ queryKey: qk.settings.allExecutorProfiles() });
  queryClient.invalidateQueries({ queryKey: qk.settings.executors() });
  queryClient.invalidateQueries({ queryKey: qk.settings.executorProfiles(payload.executor_id) });
}

function removeExecutorProfile(queryClient: QueryClient, profileId: string) {
  queryClient.setQueriesData<{ profiles: Array<{ id: string }> }>(
    { queryKey: ["settings", "executors"] },
    (prev) =>
      prev?.profiles
        ? {
            ...prev,
            profiles: prev.profiles.filter((profile) => profile.id !== profileId),
          }
        : prev,
  );
}

function patchTaskPr(queryClient: QueryClient, pr: TaskPR) {
  if (!pr.task_id) return;
  queryClient.setQueryData<TaskPR[]>(qk.integrations.github.taskPr(pr.task_id), (prev) =>
    upsertBy(prev ?? [], pr, (item) => `${item.repository_id ?? ""}:${item.pr_number}`),
  );
}

function patchTaskCiOptions(queryClient: QueryClient, options: TaskCIAutomationOptions) {
  if (!options.task_id) return;
  queryClient.setQueryData(qk.integrations.github.taskCiOptions(options.task_id), options);
}

function upsertSecret(queryClient: QueryClient, item: SecretListItem) {
  queryClient.setQueryData<SecretListItem[]>(qk.settings.secrets(), (prev) =>
    upsertBy(prev ?? [], item, (secret) => secret.id),
  );
}

function patchSystemJob(queryClient: QueryClient, job: SystemJob) {
  queryClient.setQueryData(qk.system.job(job.id), job);
  queryClient.setQueryData<Record<string, SystemJob>>(qk.system.jobs(), (prev) => ({
    ...(prev ?? {}),
    [job.id]: job,
  }));
}

function patchGitHubStatusRateLimit(queryClient: QueryClient, update: GitHubRateLimitUpdate) {
  queryClient.setQueryData(qk.integrations.github.status(), (prev: unknown) => {
    if (!prev || typeof prev !== "object") return prev;
    const status = prev as { rate_limit?: GitHubRateLimitInfo };
    const rateLimit: GitHubRateLimitInfo = { ...(status.rate_limit ?? {}) };
    for (const snapshot of update.snapshots ?? []) {
      rateLimit[snapshot.resource] = snapshot;
    }
    return { ...status, rate_limit: rateLimit };
  });
}

function upsertBy<T>(items: T[], next: T, keyOf: (item: T) => string): T[] {
  const key = keyOf(next);
  let replaced = false;
  const result = items.map((item) => {
    if (keyOf(item) !== key) return item;
    replaced = true;
    return next;
  });
  return replaced ? result : [...result, next];
}
