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

export type ExecutorType = 'local_pc' | 'local_docker' | 'remote_docker' | 'remote_vps' | 'k8s';
export type ExecutorStatus = 'active' | 'disabled';

export type Executor = {
  id: string;
  name: string;
  type: ExecutorType;
  status: ExecutorStatus;
  isSystem: boolean;
  config: Record<string, string>;
};

export type EnvironmentKind = 'local_pc' | 'docker_image';
export type BaseDocker = 'universal' | 'golang' | 'node' | 'python';

export type EnvironmentBuildConfig = {
  baseImage: BaseDocker;
  installAgents: AgentType[];
};

export type Environment = {
  id: string;
  name: string;
  kind: EnvironmentKind;
  worktreeRoot?: string;
  imageTag?: string;
  dockerfile?: string;
  buildConfig?: EnvironmentBuildConfig;
};

export type AgentType = 'claude-code' | 'codex' | 'auggie';

export type AgentProfile = {
  id: string;
  agent: AgentType;
  name: string;
  agentDisplayName: string;
  model: string;
  autoApprove: boolean;
  temperature: number;
};

export type SettingsData = {
  general: GeneralSettings;
  workspaces: Workspace[];
  environments: Environment[];
  executors: Executor[];
  agents: AgentProfile[];
};
