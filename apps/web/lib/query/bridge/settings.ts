import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "@/lib/query/keys";
import { toAgentProfileOption } from "@/lib/types/settings";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import type { Agent, Executor, ExecutorProfile } from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type {
  ExecutorProfilePayload,
  AgentInstallOutputPayload,
  UserSettingsUpdatedPayload,
} from "@/lib/types/backend";
import type { AgentProfileOption, InstallJob, UserSettingsState } from "@/lib/types/settings";
import { DEFAULT_USER_SETTINGS } from "@/lib/types/settings";
import { parseVoiceMode } from "@/lib/ssr/user-settings";
import { wrapBridgeHandler } from "./index";

// ---------------------------------------------------------------------------
// Install job helpers (mirrors settings-slice.ts semantics exactly)
// ---------------------------------------------------------------------------

const MAX_OUTPUT_BYTES = 64 * 1024;

function installJobStartedAtMs(job: Pick<InstallJob, "started_at">): number {
  return Date.parse(job.started_at);
}

function upsertJobInMap(
  byAgent: Record<string, InstallJob>,
  job: InstallJob,
): Record<string, InstallJob> {
  const current = byAgent[job.agent_name];
  // Drop stale events from a previous job_id (e.g. after retry).
  if (
    current &&
    current.job_id !== job.job_id &&
    installJobStartedAtMs(current) > installJobStartedAtMs(job)
  ) {
    return byAgent;
  }
  return { ...byAgent, [job.agent_name]: job };
}

function appendOutputInMap(
  byAgent: Record<string, InstallJob>,
  payload: AgentInstallOutputPayload,
): Record<string, InstallJob> {
  const current = byAgent[payload.agent_name];
  if (!current) return byAgent;
  const next = (current.output ?? "") + payload.chunk;
  const capped = next.length > MAX_OUTPUT_BYTES ? next.slice(next.length - MAX_OUTPUT_BYTES) : next;
  return { ...byAgent, [payload.agent_name]: { ...current, output: capped } };
}

function jobsToByAgent(jobs: InstallJob[]): Record<string, InstallJob> {
  const byAgent: Record<string, InstallJob> = {};
  for (const job of jobs) {
    const existing = byAgent[job.agent_name];
    if (!existing || installJobStartedAtMs(job) > installJobStartedAtMs(existing)) {
      byAgent[job.agent_name] = job;
    }
  }
  return byAgent;
}

// ---------------------------------------------------------------------------
// Agent profile helpers
// ---------------------------------------------------------------------------

function buildProfileOption(agents: Agent[], rawProfile: unknown): AgentProfileOption | null {
  const normalized = normalizeAgentProfile(rawProfile);
  const raw = (rawProfile ?? {}) as Record<string, unknown>;
  const agentId = typeof raw.agent_id === "string" ? raw.agent_id : "";
  const agent = agents.find((a) => a.id === agentId);
  const stub = {
    id: agentId,
    name: agent?.name ?? "",
    capability_status: agent?.capability_status,
    capability_error: agent?.capability_error,
  };
  return toAgentProfileOption(stub, normalized);
}

// ---------------------------------------------------------------------------
// Executor profile normalizer (mirrors executor-profiles.ts handler)
// ---------------------------------------------------------------------------

function toProfile(payload: ExecutorProfilePayload): ExecutorProfile {
  return {
    id: payload.id,
    executor_id: payload.executor_id,
    name: payload.name,
    mcp_policy: payload.mcp_policy,
    config: payload.config,
    prepare_script: payload.prepare_script,
    cleanup_script: payload.cleanup_script,
    created_at: payload.created_at ?? new Date().toISOString(),
    updated_at: payload.updated_at ?? new Date().toISOString(),
  };
}

// ---------------------------------------------------------------------------
// Sub-registrars
// ---------------------------------------------------------------------------

function registerExecutorHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "executor.created",
      wrapBridgeHandler(qc, "executor.created", (message) => {
        const p = message.payload;
        const executor: Executor = {
          id: p.id,
          name: p.name,
          type: p.type,
          status: p.status,
          is_system: p.is_system,
          config: p.config,
          created_at: p.created_at ?? new Date().toISOString(),
          updated_at: p.updated_at ?? new Date().toISOString(),
        };
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          const items = prev ?? [];
          return [...items.filter((e) => e.id !== executor.id), executor];
        });
      }),
    ),

    ws.on(
      "executor.updated",
      wrapBridgeHandler(qc, "executor.updated", (message) => {
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          if (!prev) return prev;
          return prev.map((e) => (e.id === message.payload.id ? { ...e, ...message.payload } : e));
        });
      }),
    ),

    ws.on(
      "executor.deleted",
      wrapBridgeHandler(qc, "executor.deleted", (message) => {
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          if (!prev) return prev;
          return prev.filter((e) => e.id !== message.payload.id);
        });
      }),
    ),
  ];
}

function registerExecutorProfileHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "executor.profile.created",
      wrapBridgeHandler(qc, "executor.profile.created", (message) => {
        const profile = toProfile(message.payload as ExecutorProfilePayload);
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          if (!prev) return prev;
          return prev.map((exec) =>
            exec.id === profile.executor_id
              ? { ...exec, profiles: [...(exec.profiles ?? []), profile] }
              : exec,
          );
        });
      }),
    ),

    ws.on(
      "executor.profile.updated",
      wrapBridgeHandler(qc, "executor.profile.updated", (message) => {
        const profile = toProfile(message.payload as ExecutorProfilePayload);
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          if (!prev) return prev;
          return prev.map((exec) =>
            exec.id === profile.executor_id
              ? {
                  ...exec,
                  profiles: (exec.profiles ?? []).map((p) => (p.id === profile.id ? profile : p)),
                }
              : exec,
          );
        });
      }),
    ),

    ws.on(
      "executor.profile.deleted",
      wrapBridgeHandler(qc, "executor.profile.deleted", (message) => {
        const { id } = message.payload;
        qc.setQueryData(qk.settings.executors(), (prev: Executor[] | undefined) => {
          if (!prev) return prev;
          return prev.map((exec) => ({
            ...exec,
            profiles: (exec.profiles ?? []).filter((p) => p.id !== id),
          }));
        });
      }),
    ),

    // Executor prepare — session-runtime concern (wave 5), no-op here.
    ws.on(
      "executor.prepare.progress",
      wrapBridgeHandler(qc, "executor.prepare.progress", (_message) => {
        // wave 5 — Zustand still handles this
      }),
    ),

    ws.on(
      "executor.prepare.completed",
      wrapBridgeHandler(qc, "executor.prepare.completed", (_message) => {
        // wave 5 — Zustand still handles this
      }),
    ),
  ];
}

function registerSecretHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "secrets.created",
      wrapBridgeHandler(qc, "secrets.created", (message) => {
        const item = message.payload;
        qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
          const items = prev ?? [];
          return [...items.filter((s) => s.id !== item.id), item];
        });
      }),
    ),

    ws.on(
      "secrets.updated",
      wrapBridgeHandler(qc, "secrets.updated", (message) => {
        const item = message.payload;
        qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
          if (!prev) return prev;
          return prev.map((s) => (s.id === item.id ? { ...s, ...item } : s));
        });
      }),
    ),

    ws.on(
      "secrets.deleted",
      wrapBridgeHandler(qc, "secrets.deleted", (message) => {
        const { id } = message.payload;
        qc.setQueryData(qk.settings.secrets(), (prev: SecretListItem[] | undefined) => {
          if (!prev) return prev;
          return prev.filter((s) => s.id !== id);
        });
      }),
    ),
  ];
}

function registerAgentHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "agent.available.updated",
      wrapBridgeHandler(qc, "agent.available.updated", (message) => {
        qc.setQueryData(qk.settings.availableAgents(), () => ({
          agents: message.payload.agents ?? [],
          tools: message.payload.tools ?? [],
        }));
      }),
    ),

    ws.on(
      "agent.install.started",
      wrapBridgeHandler(qc, "agent.install.started", (message) => {
        const job = message.payload as unknown as InstallJob;
        qc.setQueryData(qk.settings.installJobs(), (prev: InstallJob[] | undefined) => {
          return Object.values(upsertJobInMap(jobsToByAgent(prev ?? []), job));
        });
      }),
    ),

    ws.on(
      "agent.install.output",
      wrapBridgeHandler(qc, "agent.install.output", (message) => {
        const payload = message.payload as unknown as AgentInstallOutputPayload;
        qc.setQueryData(qk.settings.installJobs(), (prev: InstallJob[] | undefined) => {
          if (!prev) return prev;
          return Object.values(appendOutputInMap(jobsToByAgent(prev), payload));
        });
      }),
    ),

    ws.on(
      "agent.install.finished",
      wrapBridgeHandler(qc, "agent.install.finished", (message) => {
        const job = message.payload as unknown as InstallJob;
        qc.setQueryData(qk.settings.installJobs(), (prev: InstallJob[] | undefined) => {
          return Object.values(upsertJobInMap(jobsToByAgent(prev ?? []), job));
        });
        void qc.invalidateQueries({ queryKey: qk.settings.agents() });
      }),
    ),

    ws.on(
      "agent.updated",
      wrapBridgeHandler(qc, "agent.updated", (_message) => {
        void qc.invalidateQueries({ queryKey: qk.settings.agents() });
      }),
    ),
  ];
}

function registerAgentProfileHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "agent.profile.created",
      wrapBridgeHandler(qc, "agent.profile.created", (message) => {
        const rawProfile = message.payload.profile;
        const normalized = normalizeAgentProfile(rawProfile);
        const agentId = rawProfile.agent_id;

        qc.setQueryData(qk.settings.agents(), (prev: Agent[] | undefined) => {
          if (!prev) return prev;
          return prev.map((item) =>
            item.id === agentId
              ? {
                  ...item,
                  profiles: [...item.profiles.filter((p) => p.id !== normalized.id), normalized],
                }
              : item,
          );
        });

        qc.setQueryData(qk.settings.agentProfiles(), (prev: AgentProfileOption[] | undefined) => {
          const agents: Agent[] = qc.getQueryData(qk.settings.agents()) ?? [];
          const option = buildProfileOption(agents, rawProfile);
          if (!option) return prev;
          const items = prev ?? [];
          return [...items.filter((p) => p.id !== option.id), option];
        });
      }),
    ),

    ws.on(
      "agent.profile.updated",
      wrapBridgeHandler(qc, "agent.profile.updated", (message) => {
        const rawProfile = message.payload.profile;
        const normalized = normalizeAgentProfile(rawProfile);
        const agentId = rawProfile.agent_id;

        qc.setQueryData(qk.settings.agents(), (prev: Agent[] | undefined) => {
          if (!prev) return prev;
          return prev.map((item) =>
            item.id === agentId
              ? {
                  ...item,
                  profiles: item.profiles.map((p) => (p.id === normalized.id ? normalized : p)),
                }
              : item,
          );
        });

        qc.setQueryData(qk.settings.agentProfiles(), (prev: AgentProfileOption[] | undefined) => {
          if (!prev) return prev;
          const agents: Agent[] = qc.getQueryData(qk.settings.agents()) ?? [];
          const option = buildProfileOption(agents, rawProfile);
          if (!option) return prev;
          return prev.map((p) => (p.id === option.id ? option : p));
        });
      }),
    ),

    ws.on(
      "agent.profile.deleted",
      wrapBridgeHandler(qc, "agent.profile.deleted", (message) => {
        const rawProfile = message.payload.profile;
        const profileId = rawProfile.id;
        const agentId = rawProfile.agent_id;

        qc.setQueryData(qk.settings.agents(), (prev: Agent[] | undefined) => {
          if (!prev) return prev;
          return prev.map((item) =>
            item.id === agentId
              ? { ...item, profiles: item.profiles.filter((p) => p.id !== profileId) }
              : item,
          );
        });

        qc.setQueryData(qk.settings.agentProfiles(), (prev: AgentProfileOption[] | undefined) => {
          if (!prev) return prev;
          return prev.filter((p) => p.id !== profileId);
        });
      }),
    ),
  ];
}

/**
 * Merge a `user.settings.updated` WS payload onto the existing mapped
 * user-settings cache. Preserves navigation fields (workspaceId, workflowId,
 * repositoryIds) — these are SSR/user-action controlled, not broadcast — by
 * keeping whatever the base already holds.
 */
function userSettingsScalarFields(p: UserSettingsUpdatedPayload) {
  return {
    preferredShell: p.preferred_shell || null,
    defaultEditorId: p.default_editor_id || null,
    enablePreviewOnClick: p.enable_preview_on_click ?? false,
    chatSubmitKey: (p.chat_submit_key as "enter" | "cmd_enter") ?? "cmd_enter",
    reviewAutoMarkOnScroll: p.review_auto_mark_on_scroll ?? true,
    showReleaseNotification: p.show_release_notification ?? true,
    releaseNotesLastSeenVersion: (p.release_notes_last_seen_version as string) || null,
    defaultUtilityAgentId: p.default_utility_agent_id || null,
    terminalLinkBehavior: (p.terminal_link_behavior === "browser_panel"
      ? "browser_panel"
      : "new_tab") as "new_tab" | "browser_panel",
    changesPanelLayout: (p.changes_panel_layout === "tree" ? "tree" : "flat") as "flat" | "tree",
  };
}

function userSettingsCollectionFields(p: UserSettingsUpdatedPayload) {
  return {
    lspAutoStartLanguages: p.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: p.lsp_auto_install_languages ?? [],
    savedLayouts: p.saved_layouts ?? [],
    keyboardShortcuts: p.keyboard_shortcuts ?? {},
  };
}

function applyUserSettingsUpdate(
  prev: UserSettingsState | undefined,
  p: UserSettingsUpdatedPayload,
): UserSettingsState {
  return {
    ...(prev ?? DEFAULT_USER_SETTINGS),
    ...userSettingsScalarFields(p),
    ...userSettingsCollectionFields(p),
    voiceMode: parseVoiceMode(p.voice_mode),
    loaded: true,
  };
}

function registerUserSettingsHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  return [
    ws.on(
      "user.settings.updated",
      wrapBridgeHandler(qc, "user.settings.updated", (message) => {
        qc.setQueryData<UserSettingsState>(qk.settings.userSettings(), (prev) =>
          applyUserSettingsUpdate(prev, message.payload),
        );
      }),
    ),
  ];
}

// ---------------------------------------------------------------------------
// Bridge registrar
// ---------------------------------------------------------------------------

export function registerSettingsBridge(ws: WebSocketClient, qc: QueryClient): () => void {
  const unsubs: Array<() => void> = [
    ...registerExecutorHandlers(ws, qc),
    ...registerExecutorProfileHandlers(ws, qc),
    ...registerSecretHandlers(ws, qc),
    ...registerAgentHandlers(ws, qc),
    ...registerAgentProfileHandlers(ws, qc),
    ...registerUserSettingsHandlers(ws, qc),
  ];

  return () => {
    for (const unsub of unsubs) unsub();
  };
}
