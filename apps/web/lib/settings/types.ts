export type Theme = 'system' | 'light' | 'dark';
export type Editor = 'vscode' | 'cursor' | 'zed' | 'vim' | 'custom';

export type Notifications = {
  taskUpdates: boolean;
  agentCompletion: boolean;
  errors: boolean;
};

export type GeneralSettings = {
  theme: Theme;
  editor: Editor;
  customEditorCommand?: string;
  notifications: Notifications;
  backendUrl?: string;
};

export type CustomScript = {
  id: string;
  name: string;
  command: string;
};

export type Repository = {
  id: string;
  name: string;
  path: string;
  setupScript: string;
  cleanupScript: string;
  customScripts: CustomScript[];
};

export type Column = {
  id: string;
  title: string;
  color: string;
};

export type Context = {
  id: string;
  name: string;
  columns: Column[];
};

export type Workspace = {
  id: string;
  name: string;
  repositories: Repository[];
  contexts: Context[];
};

export type KeyValue = {
  id: string;
  key: string;
  value: string;
};

export type EnvironmentType = 'local-docker' | 'remote-docker';
export type BaseDocker = 'universal' | 'golang';

export type Environment = {
  id: string;
  name: string;
  type: EnvironmentType;
  baseDocker: BaseDocker;
  envVariables: KeyValue[];
  secrets: KeyValue[];
  setupScript: string;
  installedAgents: AgentType[];
};

export type AgentType = 'claude-code' | 'codex' | 'auggie';

export type AgentProfile = {
  id: string;
  agent: AgentType;
  name: string;
  model: string;
  autoApprove: boolean;
  temperature: number;
};

export type SettingsData = {
  general: GeneralSettings;
  workspaces: Workspace[];
  environments: Environment[];
  agents: AgentProfile[];
};
