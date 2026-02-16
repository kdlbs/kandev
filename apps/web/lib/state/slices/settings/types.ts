import type {
  Agent,
  AvailableAgent,
  AgentDiscovery,
  CustomPrompt,
  Environment,
  EditorOption,
  Executor,
  NotificationProvider,
} from '@/lib/types/http';

export type ExecutorsState = {
  items: Executor[];
};

export type EnvironmentsState = {
  items: Environment[];
};

export type SettingsAgentsState = {
  items: Agent[];
};

export type AgentDiscoveryState = {
  items: AgentDiscovery[];
};

export type AvailableAgentsState = {
  items: AvailableAgent[];
  loading: boolean;
  loaded: boolean;
};

export type AgentProfileOption = {
  id: string;
  label: string;
  agent_id: string;
  agent_name: string;
};

export type AgentProfilesState = {
  items: AgentProfileOption[];
  version: number;
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

export type NotificationProvidersState = {
  items: NotificationProvider[];
  events: string[];
  appriseAvailable: boolean;
  loaded: boolean;
  loading: boolean;
};

export type SettingsDataState = {
  executorsLoaded: boolean;
  environmentsLoaded: boolean;
  agentsLoaded: boolean;
};

export type UserSettingsState = {
  workspaceId: string | null;
  boardId: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: 'enter' | 'cmd_enter';
  reviewAutoMarkOnScroll: boolean;
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  lspServerConfigs: Record<string, Record<string, unknown>>;
  loaded: boolean;
};

export type SettingsSliceState = {
  executors: ExecutorsState;
  environments: EnvironmentsState;
  settingsAgents: SettingsAgentsState;
  agentDiscovery: AgentDiscoveryState;
  availableAgents: AvailableAgentsState;
  agentProfiles: AgentProfilesState;
  editors: EditorsState;
  prompts: PromptsState;
  notificationProviders: NotificationProvidersState;
  settingsData: SettingsDataState;
  userSettings: UserSettingsState;
};

export type SettingsSliceActions = {
  setExecutors: (executors: ExecutorsState['items']) => void;
  setEnvironments: (environments: EnvironmentsState['items']) => void;
  setSettingsAgents: (agents: SettingsAgentsState['items']) => void;
  setAgentDiscovery: (agents: AgentDiscoveryState['items']) => void;
  setAvailableAgents: (agents: AvailableAgentsState['items']) => void;
  setAvailableAgentsLoading: (loading: boolean) => void;
  setAgentProfiles: (profiles: AgentProfilesState['items']) => void;
  setEditors: (editors: EditorsState['items']) => void;
  setEditorsLoading: (loading: boolean) => void;
  setPrompts: (prompts: PromptsState['items']) => void;
  setPromptsLoading: (loading: boolean) => void;
  setNotificationProviders: (state: NotificationProvidersState) => void;
  setNotificationProvidersLoading: (loading: boolean) => void;
  setSettingsData: (next: Partial<SettingsDataState>) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  bumpAgentProfilesVersion: () => void;
};

export type SettingsSlice = SettingsSliceState & SettingsSliceActions;
