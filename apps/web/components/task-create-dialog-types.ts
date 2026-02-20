import type {
  LocalRepository,
  Repository,
  Workspace,
  Environment,
  Executor,
  Branch,
  Task,
} from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorOptions,
} from "@/components/task-create-dialog-options";
import type { useToast } from "@/components/toast-provider";

export type StepType = {
  id: string;
  title: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
};

export type TaskCreateDialogInitialValues = {
  title: string;
  description?: string;
  repositoryId?: string;
  branch?: string;
  state?: Task["state"];
};

export type StoreSelections = {
  agentProfiles: AgentProfileOption[];
  environments: Environment[];
  executors: Executor[];
  workspaceDefaults: Workspace | null | undefined;
};

export type DialogComputedValues = {
  isPassthroughProfile: boolean;
  effectiveWorkflowId: string | null;
  effectiveDefaultStepId: string | null;
  workspaceDefaults: Workspace | null | undefined;
  hasRepositorySelection: boolean;
  branchOptions: ReturnType<typeof useBranchOptions>;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorOptions: ReturnType<typeof useExecutorOptions>;
  executorHint: string | null;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>["headerRepositoryOptions"];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
};

export type DialogComputedArgs = {
  fs: DialogFormState;
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  branches: Branch[];
  settingsData: { agentsLoaded: boolean; executorsLoaded: boolean };
  agentProfiles: AgentProfileOption[];
  workspaces: Workspace[];
  executors: Executor[];
  repositories: Repository[];
};

export type TaskCreateEffectsArgs = {
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  repositories: Repository[];
  repositoriesLoading: boolean;
  branches: Branch[];
  agentProfiles: AgentProfileOption[];
  environments: Environment[];
  executors: Executor[];
  workspaceDefaults: Workspace | null | undefined;
  toast: ReturnType<typeof useToast>["toast"];
};

export type DialogFormState = {
  taskName: string;
  setTaskName: (v: string) => void;
  hasTitle: boolean;
  setHasTitle: (v: boolean) => void;
  hasDescription: boolean;
  setHasDescription: (v: boolean) => void;
  descriptionInputRef: import("react").RefObject<{ getValue: () => string } | null>;
  repositoryId: string;
  setRepositoryId: (v: string) => void;
  branch: string;
  setBranch: (v: string) => void;
  agentProfileId: string;
  setAgentProfileId: (v: string) => void;
  environmentId: string;
  setEnvironmentId: (v: string) => void;
  executorId: string;
  setExecutorId: (v: string) => void;
  discoveredRepositories: LocalRepository[];
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
  discoveredRepoPath: string;
  setDiscoveredRepoPath: (v: string) => void;
  selectedLocalRepo: LocalRepository | null;
  setSelectedLocalRepo: (v: LocalRepository | null) => void;
  localBranches: Branch[];
  setLocalBranches: (v: Branch[]) => void;
  localBranchesLoading: boolean;
  setLocalBranchesLoading: (v: boolean) => void;
  discoverReposLoading: boolean;
  setDiscoverReposLoading: (v: boolean) => void;
  discoverReposLoaded: boolean;
  setDiscoverReposLoaded: (v: boolean) => void;
  selectedWorkflowId: string | null;
  setSelectedWorkflowId: (v: string | null) => void;
  fetchedSteps: StepType[] | null;
  setFetchedSteps: (v: StepType[] | null) => void;
  isCreatingSession: boolean;
  setIsCreatingSession: (v: boolean) => void;
  isCreatingTask: boolean;
  setIsCreatingTask: (v: boolean) => void;
};
