import type {
  Agent,
  AgentProfile,
  CapabilityStatus,
  MCPTaskAgentProfileDefault,
  SavedLayout,
} from "@/lib/types/http";
import type {
  VoiceInputActivationMode,
  VoiceInputEngine,
  WhisperWebModelSize,
} from "@/lib/types/http-voice";
import type { SidebarView, SidebarViewDraft } from "@/lib/state/slices/ui/sidebar-view-types";
import type { SidebarTaskPrefsState } from "@/lib/state/slices/ui/types";
import type { TasksListGroup, TasksListSort } from "@/lib/tasks/tasks-list-options";

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

/** Single source of truth for mapping an API Agent+Profile to a store AgentProfileOption. */
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

export type UserSettingsState = {
  workspaceId: string | null;
  kanbanViewMode: string | null;
  workflowId: string | null;
  repositoryIds: string[];
  tasksListSort: TasksListSort;
  tasksListGroup: TasksListGroup;
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: "enter" | "cmd_enter";
  reviewAutoMarkOnScroll: boolean;
  confirmTaskArchive: boolean;
  mcpTaskAgentProfileDefault: MCPTaskAgentProfileDefault;
  showReleaseNotification: boolean;
  releaseNotesLastSeenVersion: string | null;
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  lspServerConfigs: Record<string, Record<string, unknown>>;
  savedLayouts: SavedLayout[];
  sidebarViews: SidebarView[];
  sidebarActiveViewId: string | null;
  sidebarDraft: SidebarViewDraft | null;
  sidebarTaskPrefs: SidebarTaskPrefsState;
  taskCreateLastUsed: TaskCreateLastUsedState;
  jiraSavedViews: unknown;
  jiraTaskPresets: unknown;
  githubSavedPresets: unknown;
  githubDefaultQueryPresets: unknown;
  gitlabSavedPresets: unknown;
  defaultUtilityAgentId: string | null;
  keyboardShortcuts: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminalLinkBehavior: "new_tab" | "browser_panel";
  terminalFontFamily: string | null;
  terminalFontSize: number | null;
  changesPanelLayout: "flat" | "tree";
  systemMetricsDisplay: { showInTopbar: boolean };
  voiceMode: VoiceModeState;
  loaded: boolean;
};

export type TaskCreateLastUsedState = {
  repositoryId: string | null;
  branch: string | null;
  agentProfileId: string | null;
  executorProfileId: string | null;
  synced?: boolean;
};

export type VoiceModeState = {
  enabled: boolean;
  engine: VoiceInputEngine;
  language: string;
  mode: VoiceInputActivationMode;
  autoSend: boolean;
  whisperWebModel: WhisperWebModelSize;
};

/** Default values used by the slice init and by SSR hydration fallback. */
export const DEFAULT_VOICE_MODE_STATE: VoiceModeState = {
  enabled: true,
  engine: "auto",
  language: "auto",
  mode: "toggle",
  autoSend: false,
  whisperWebModel: "base",
};

export type SettingsSliceState = {
  userSettings: UserSettingsState;
};

export type SettingsSliceActions = {
  setUserSettings: (settings: UserSettingsState) => void;
};

export type SettingsSlice = SettingsSliceState & SettingsSliceActions;
