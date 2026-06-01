import type { Agent, AgentProfile, CapabilityStatus, SavedLayout } from "@/lib/types/http";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import type {
  VoiceInputActivationMode,
  VoiceInputEngine,
  WhisperWebModelSize,
} from "@/lib/types/http-voice";

/**
 * Portable settings-domain types and helpers shared by the TanStack Query
 * layer (query-options, bridge), SSR builders, and read hooks. These used to
 * live in the now-removed `lib/state/slices/settings` Zustand mirror; they are
 * pure data shapes with no Zustand dependency.
 */

export type AgentProfileOption = {
  id: string;
  label: string;
  agent_id: string;
  agent_name: string;
  cli_passthrough: boolean;
  /**
   * Host utility probe status for the agent this profile belongs to.
   * Used by pickers and the settings sidebar to flag profiles whose agent
   * needs login or reinstallation.
   */
  capability_status?: CapabilityStatus;
  capability_error?: string;
};

/** Single source of truth for mapping an API Agent+Profile to an AgentProfileOption. */
export function toAgentProfileOption(
  agent: Pick<Agent, "id" | "name" | "capability_status" | "capability_error">,
  profile: Pick<AgentProfile, "id" | "agentDisplayName" | "name"> & { cliPassthrough?: boolean },
): AgentProfileOption {
  return {
    id: profile.id,
    label: `${profile.agentDisplayName ?? ""} • ${profile.name}`,
    agent_id: agent.id,
    agent_name: agent.name,
    cli_passthrough: profile.cliPassthrough ?? false,
    capability_status: agent.capability_status,
    capability_error: agent.capability_error,
  };
}

/**
 * Mapped (camelCase) Voice Mode user settings. Server-persisted under
 * `user_settings.voice_mode`; arrives over the user WS payload as `voice_mode`
 * and is read from the TQ user-settings cache by the voice components.
 */
export type VoiceModeState = {
  enabled: boolean;
  engine: VoiceInputEngine;
  language: string;
  mode: VoiceInputActivationMode;
  autoSend: boolean;
  whisperWebModel: WhisperWebModelSize;
};

/** Default values used by SSR hydration fallback and the TQ default settings. */
export const DEFAULT_VOICE_MODE_STATE: VoiceModeState = {
  enabled: true,
  engine: "auto",
  language: "auto",
  mode: "toggle",
  autoSend: false,
  whisperWebModel: "base",
};

export type InstallJobStatus = "queued" | "running" | "succeeded" | "failed";

export type InstallJob = {
  job_id: string;
  agent_name: string;
  status: InstallJobStatus;
  output?: string;
  error?: string;
  exit_code?: number;
  started_at: string;
  finished_at?: string;
};

/**
 * Mapped user settings (camelCase). Same shape the old Zustand slice exposed;
 * now produced by `mapUserSettingsResponse` and read from the TQ cache.
 */
export type UserSettingsState = {
  workspaceId: string | null;
  kanbanViewMode: string | null;
  workflowId: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: "enter" | "cmd_enter";
  reviewAutoMarkOnScroll: boolean;
  showReleaseNotification: boolean;
  releaseNotesLastSeenVersion: string | null;
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  lspServerConfigs: Record<string, Record<string, unknown>>;
  savedLayouts: SavedLayout[];
  sidebarViews: SidebarView[];
  defaultUtilityAgentId: string | null;
  keyboardShortcuts: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminalLinkBehavior: "new_tab" | "browser_panel";
  terminalFontFamily: string | null;
  terminalFontSize: number | null;
  changesPanelLayout: "flat" | "tree";
  voiceMode: VoiceModeState;
  loaded: boolean;
};

/** Default (unloaded) user-settings shape — replaces the old Zustand default. */
export const DEFAULT_USER_SETTINGS: UserSettingsState = {
  workspaceId: null,
  kanbanViewMode: null,
  workflowId: null,
  repositoryIds: [],
  preferredShell: null,
  shellOptions: [],
  defaultEditorId: null,
  enablePreviewOnClick: false,
  terminalLinkBehavior: "new_tab",
  chatSubmitKey: "cmd_enter",
  reviewAutoMarkOnScroll: true,
  showReleaseNotification: true,
  releaseNotesLastSeenVersion: null,
  lspAutoStartLanguages: [],
  lspAutoInstallLanguages: [],
  lspServerConfigs: {},
  savedLayouts: [],
  sidebarViews: [],
  defaultUtilityAgentId: null,
  keyboardShortcuts: {},
  terminalFontFamily: null,
  terminalFontSize: null,
  changesPanelLayout: "flat",
  voiceMode: { ...DEFAULT_VOICE_MODE_STATE },
  loaded: false,
};
