import type {
  Agent,
  AgentProfile,
  AvailableAgent,
  AgentDiscovery,
  CapabilityStatus,
  CustomPrompt,
  EditorOption,
  Executor,
  NotificationProvider,
  SavedLayout,
  ToolStatus,
  LspStatusLocation,
  MCPTaskAgentProfileDefault,
  StartupPage,
} from "@/lib/types/http";
import type { SidebarView, SidebarViewDraft } from "@/lib/state/slices/ui/sidebar-view-types";
import type { SidebarTaskPrefsState } from "@/lib/state/slices/ui/types";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { SpritesStatus, SpritesInstance } from "@/lib/types/http-sprites";
import type { TasksListGroup, TasksListSort } from "@/lib/tasks/tasks-list-options";
import type { SleepInhibitionResponse } from "@/lib/types/system";

export type ExecutorsState = {
  items: Executor[];
};

export type SettingsAgentsState = {
  items: Agent[];
};

export type AgentDiscoveryState = {
  items: AgentDiscovery[];
  loading: boolean;
  loaded: boolean;
};

export type AvailableAgentsState = {
  items: AvailableAgent[];
  tools: ToolStatus[];
  loading: boolean;
  loaded: boolean;
};

export type AgentProfileOption = {
  id: string;
  label: string;
  agent_id: string;
  agent_name: string;
  cli_passthrough: boolean;
  /** Configured start model (ACP model ID). Empty = agent default. */
  model?: string;
  /** Optional explicit fallback model; ignored when auto_fallback is on. */
  fallback_model?: string;
  /** Legacy automatic-fallback opt-in. */
  auto_fallback?: boolean;
  workspace_id?: string;
  /** Persisted profile revision (RFC3339 updated_at), used to prefer newer
   * WS-delivered options over a stale in-flight response. */
  updatedAt?: string;
  /**
   * False hides the profile from task/session creation pickers. Existing
   * sessions keep their labels and the profile stays editable in settings.
   */
  enabled?: boolean;
  /**
   * Host utility probe status for the agent this profile belongs to.
   * Used by pickers and the settings sidebar to flag profiles whose agent
   * needs login or reinstallation.
   */
  capability_status?: CapabilityStatus;
  capability_error?: string;
};

/** Profiles with an omitted enabled field remain selectable for compatibility. */
export function isSelectableAgentProfile(profile: Pick<AgentProfileOption, "enabled">): boolean {
  return profile.enabled !== false;
}

/**
 * Compares two RFC3339 timestamps as instants (millisecond precision).
 * Lexical comparison misorders values with fractional seconds or differing
 * offsets (e.g. `10:00:00.100Z` is newer than `10:00:00Z` but sorts before
 * it), so revision precedence must parse the instants. Missing timestamps
 * sort oldest so a present timestamp always wins.
 */
export function compareTimestamps(a: string | undefined, b: string | undefined): number {
  if (a === b) return 0;
  if (a === undefined || a === "") return -1;
  if (b === undefined || b === "") return 1;
  const aMs = Date.parse(a);
  const bMs = Date.parse(b);
  if (Number.isNaN(aMs)) return -1;
  if (Number.isNaN(bMs)) return 1;
  return aMs - bMs;
}

/**
 * Merge two option lists by ID, keeping the NEWER revision per id (missing or
 * equal updatedAt defers to the rebuilt option). A newer WebSocket-delivered
 * option must never be regressed by a stale list rebuild — e.g. a disabled
 * profile whose option was updated by `agent.profile.updated` while its owning
 * agent was absent must not reappear as selectable.
 */
export function mergeOptionsByNewest(
  previous: AgentProfileOption[],
  rebuilt: AgentProfileOption[],
): AgentProfileOption[] {
  const byId = new Map<string, AgentProfileOption>();
  for (const option of previous) {
    byId.set(option.id, option);
  }
  for (const option of rebuilt) {
    const existing = byId.get(option.id);
    if (!existing || compareTimestamps(option.updatedAt, existing.updatedAt) >= 0) {
      byId.set(option.id, option);
    }
  }
  return [...byId.values()];
}

/** Single source of truth for mapping an API Agent+Profile to a store AgentProfileOption. */
export function toAgentProfileOption(
  agent: Pick<Agent, "id" | "name" | "capability_status" | "capability_error">,
  profile: Pick<AgentProfile, "id" | "agentDisplayName" | "name" | "workspaceId"> & {
    updatedAt?: string;
    cliPassthrough?: boolean;
    model?: string;
    fallbackModel?: string;
    autoFallback?: boolean;
    enabled?: boolean;
  },
): AgentProfileOption {
  return {
    id: profile.id,
    label: `${profile.agentDisplayName ?? ""} • ${profile.name}`,
    agent_id: agent.id,
    agent_name: agent.name,
    cli_passthrough: profile.cliPassthrough ?? false,
    model: profile.model ?? undefined,
    fallback_model: profile.fallbackModel ?? undefined,
    auto_fallback: profile.autoFallback ?? undefined,
    workspace_id: profile.workspaceId,
    updatedAt: profile.updatedAt,
    enabled: profile.enabled ?? true,
    capability_status: agent.capability_status,
    capability_error: agent.capability_error,
  };
}

export type AgentProfilesState = {
  items: AgentProfileOption[];
  version: number;
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
 * Tracks active and recently-finished install jobs by agent_name. Rehydrated
 * on page mount from GET /agent-install/jobs and kept live via WS events
 * (agent.install.started/output/finished).
 */
export type InstallJobsState = {
  byAgent: Record<string, InstallJob>;
};

export type AgentUpdateJobStatus =
  | "queued"
  | "resolving"
  | "updating"
  | "refreshing"
  | "succeeded"
  | "failed";

export type AgentUpdateJob = {
  job_id: string;
  agent_name: string;
  status: AgentUpdateJobStatus;
  current_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentUpdateJobsState = {
  byAgent: Record<string, AgentUpdateJob>;
};

export type EditorsState = {
  items: EditorOption[];
  loaded: boolean;
  loading: boolean;
};

export type PromptsState = {
  items: CustomPrompt[];
  loaded: boolean;
  loading: boolean;
};

export type SecretsState = {
  items: SecretListItem[];
  loaded: boolean;
  loading: boolean;
};

export type SpritesState = {
  status: SpritesStatus | null;
  instances: SpritesInstance[];
  loaded: boolean;
  loading: boolean;
};

export type NotificationProvidersState = {
  items: NotificationProvider[];
  events: string[];
  appriseAvailable: boolean;
  loaded: boolean;
  loading: boolean;
};

export type SettingsDataState = {
  executorsLoaded: boolean;
  agentsLoaded: boolean;
};

/** Install-wide sleep-inhibition settings and runtime status. */
export type SleepInhibitionStoreState = {
  response: SleepInhibitionResponse | null;
  loaded: boolean;
  loading: boolean;
  error: boolean;
};

export type UserSettingsState = {
  revision: number | null;
  workspaceId: string | null;
  kanbanViewMode: string | null;
  startupPage: StartupPage;
  workflowId: string | null;
  repositoryIds: string[];
  tasksListSort: TasksListSort;
  tasksListGroup: TasksListGroup;
  tasksListShowDetails: boolean;
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: "enter" | "cmd_enter";
  reviewAutoMarkOnScroll: boolean;
  confirmTaskArchive: boolean;
  preventAutoStartAgentOnOpen: boolean;
  unreadDivider: boolean;
  agentGeneratedTaskTitles: boolean;
  mcpTaskAgentProfileDefault: MCPTaskAgentProfileDefault;
  showAnchoredPromptBar: boolean;
  showScrollToLastPrompt: boolean;
  showScrollToStart: boolean;
  showTranscriptAutoScrollControl: boolean;
  showTodoListPanel: boolean;
  showTodoListPanelOnlyWhenNotEmpty: boolean;
  showReleaseNotification: boolean;
  releaseNotesLastSeenVersion: string | null;
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  lspServerConfigs: Record<string, Record<string, unknown>>;
  lspStatusLocation: LspStatusLocation;
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
  azureDevOpsBrowsePreferences: unknown;
  defaultUtilityAgentId: string | null;
  keyboardShortcuts: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminalLinkBehavior: "new_tab" | "browser_panel";
  terminalFontFamily: string | null;
  terminalFontSize: number | null;
  changesPanelLayout: "flat" | "tree";
  systemMetricsDisplay: { showInTopbar: boolean; simplified: boolean };
  appStatusBarEnabled: boolean;
  appStatusBarOrder: AppStatusBarOrderState;
  hiddenWorkflowStepIds: Record<string, string[]>;
  loaded: boolean;
};

export type AppStatusBarOrderState = {
  leftItemIds: string[];
  rightItemIds: string[];
};

export type TaskCreateLastUsedState = {
  repositoryId: string | null;
  branch: string | null;
  agentProfileId: string | null;
  executorProfileId: string | null;
  workflowIdsByWorkspace: Record<string, string>;
  synced?: boolean;
};

/** Default values used by the slice init and by SSR hydration fallback. */

export type SettingsSliceState = {
  executors: ExecutorsState;
  settingsAgents: SettingsAgentsState;
  agentDiscovery: AgentDiscoveryState;
  availableAgents: AvailableAgentsState;
  agentProfiles: AgentProfilesState;
  installJobs: InstallJobsState;
  updateJobs: AgentUpdateJobsState;
  editors: EditorsState;
  prompts: PromptsState;
  secrets: SecretsState;
  sprites: SpritesState;
  notificationProviders: NotificationProvidersState;
  settingsData: SettingsDataState;
  sleepInhibition: SleepInhibitionStoreState;
  userSettings: UserSettingsState;
};

export type SettingsSliceActions = {
  setExecutors: (executors: ExecutorsState["items"]) => void;
  setSettingsAgents: (agents: SettingsAgentsState["items"]) => void;
  setAgentDiscovery: (agents: AgentDiscoveryState["items"]) => void;
  setAgentDiscoveryLoading: (loading: boolean) => void;
  setAvailableAgents: (
    agents: AvailableAgentsState["items"],
    tools?: AvailableAgentsState["tools"],
  ) => void;
  setAvailableAgentsLoading: (loading: boolean) => void;
  setAgentProfiles: (profiles: AgentProfilesState["items"]) => void;
  setInstallJobs: (jobs: InstallJob[]) => void;
  upsertInstallJob: (job: InstallJob) => void;
  appendInstallOutput: (agentName: string, chunk: string) => void;
  clearInstallJob: (agentName: string) => void;
  setAgentUpdateJobs: (jobs: AgentUpdateJob[]) => void;
  upsertAgentUpdateJob: (job: AgentUpdateJob) => void;
  appendAgentUpdateOutput: (agentName: string, jobId: string, chunk: string) => void;
  clearAgentUpdateJob: (agentName: string) => void;
  setEditors: (editors: EditorsState["items"]) => void;
  setEditorsLoading: (loading: boolean) => void;
  setPrompts: (prompts: PromptsState["items"]) => void;
  setPromptsLoading: (loading: boolean) => void;
  setSecrets: (items: SecretsState["items"]) => void;
  setSecretsLoading: (loading: boolean) => void;
  addSecret: (item: SecretListItem) => void;
  updateSecret: (item: SecretListItem) => void;
  removeSecret: (id: string) => void;
  setSpritesStatus: (status: SpritesStatus) => void;
  setSpritesInstances: (instances: SpritesInstance[]) => void;
  setSpritesLoading: (loading: boolean) => void;
  removeSpritesInstance: (name: string) => void;
  setNotificationProviders: (state: NotificationProvidersState) => void;
  setNotificationProvidersLoading: (loading: boolean) => void;
  setSettingsData: (next: Partial<SettingsDataState>) => void;
  setSleepInhibition: (response: SleepInhibitionResponse) => void;
  setSleepInhibitionLoading: (loading: boolean) => void;
  setSleepInhibitionError: (error: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  bumpAgentProfilesVersion: () => void;
};

export type SettingsSlice = SettingsSliceState & SettingsSliceActions;
