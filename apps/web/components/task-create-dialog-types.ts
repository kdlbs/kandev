import type {
  LocalRepository,
  Repository,
  Workspace,
  Executor,
  Branch,
  Task,
} from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import type {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorProfileOptions,
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
  /** Existing remote branch to check out directly in the worktree (e.g. a PR's head branch),
   * instead of creating a new branch off `branch`. */
  checkoutBranch?: string;
  state?: Task["state"];
  /** When set, opens the dialog in GitHub URL mode pre-filled with this value
   * (e.g. "github.com/owner/repo"). Used when no matching workspace repo exists. */
  githubUrl?: string;
};

export type StoreSelections = {
  agentProfiles: AgentProfileOption[];
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
  executorProfileOptions: ReturnType<typeof useExecutorProfileOptions>;
  executorHint: string | null;
  isLocalExecutor: boolean;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>["headerRepositoryOptions"];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
  /** True when the effective workflow has an agent_profile_id override */
  workflowAgentLocked: boolean;
  /** The agent_profile_id from the effective workflow (empty string if none) */
  workflowAgentProfileId: string;
  /** User selection if any, else the workflow override; what footer/submit/passthrough should consult */
  effectiveAgentProfileId: string;
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
  workflows: Array<{ id: string; agent_profile_id?: string }>;
};

export type TaskCreateEffectsArgs = {
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  repositories: Repository[];
  repositoriesLoading: boolean;
  branches: Branch[];
  agentProfiles: AgentProfileOption[];
  executors: Executor[];
  workspaceDefaults: Workspace | null | undefined;
  toast: ReturnType<typeof useToast>["toast"];
  workflows: Array<{ id: string; agent_profile_id?: string }>;
};

import type { FileAttachment } from "@/components/task/chat/file-attachment";

export type TaskFormInputsHandle = {
  getValue: () => string;
  setValue: (v: string) => void;
  getAttachments: () => FileAttachment[];
};

export type DialogFormState = {
  taskName: string;
  setTaskName: (v: string) => void;
  hasTitle: boolean;
  setHasTitle: (v: boolean) => void;
  hasDescription: boolean;
  setHasDescription: (v: boolean) => void;
  /** Restored draft description, used as initialDescription for TaskFormInputs */
  draftDescription: string;
  /** Cycle counter incremented each time dialog opens - used in key for remount */
  openCycle: number;
  /** Computed defaults for current open cycle (includes draft restoration) */
  currentDefaults: { name: string; description: string };
  descriptionInputRef: import("react").RefObject<TaskFormInputsHandle | null>;
  repositoryId: string;
  setRepositoryId: (v: string) => void;
  branch: string;
  setBranch: (v: string) => void;
  agentProfileId: string;
  setAgentProfileId: (v: string) => void;
  executorId: string;
  setExecutorId: (v: string) => void;
  executorProfileId: string;
  setExecutorProfileId: (v: string) => void;
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
  useGitHubUrl: boolean;
  setUseGitHubUrl: (v: boolean) => void;
  githubUrl: string;
  setGitHubUrl: (v: string) => void;
  githubBranches: Branch[];
  setGitHubBranches: (v: Branch[]) => void;
  githubBranchesLoading: boolean;
  setGitHubBranchesLoading: (v: boolean) => void;
  githubUrlError: string | null;
  setGitHubUrlError: (v: string | null) => void;
  githubPrHeadBranch: string | null;
  setGitHubPrHeadBranch: (v: string | null) => void;
  /** When non-empty, the selected workflow overrides the agent profile */
  workflowAgentProfileId: string;
  setWorkflowAgentProfileId: (v: string) => void;
  /** Clear draft on successful submission (before closing dialog) */
  clearDraft: () => void;
  /** Local executor only: opt-in to discard local changes and start the task on a new branch */
  freshBranchEnabled: boolean;
  setFreshBranchEnabled: (v: boolean) => void;
  /** Currently checked-out branch in the selected local repo (for the disabled selector placeholder) */
  currentLocalBranch: string;
  setCurrentLocalBranch: (v: string) => void;
};

export type SubmitHandlersDeps = {
  isSessionMode: boolean;
  isEditMode: boolean;
  isPassthroughProfile: boolean;
  taskName: string;
  workspaceId: string | null;
  workflowId: string | null;
  effectiveWorkflowId: string | null;
  effectiveDefaultStepId: string | null;
  repositoryId: string;
  selectedLocalRepo: LocalRepository | null;
  useGitHubUrl: boolean;
  githubUrl: string;
  githubPrHeadBranch: string | null;
  branch: string;
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
  editingTask?: {
    id: string;
    title: string;
    description?: string;
    workflowStepId: string;
    state?: Task["state"];
    repositoryId?: string;
  } | null;
  onSuccess?: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string }) => void;
  onOpenChange: (open: boolean) => void;
  taskId: string | null;
  parentTaskId?: string;
  descriptionInputRef: React.RefObject<TaskFormInputsHandle | null>;
  setIsCreatingSession: (v: boolean) => void;
  setIsCreatingTask: (v: boolean) => void;
  setHasTitle: (v: boolean) => void;
  setHasDescription: (v: boolean) => void;
  setTaskName: (v: string) => void;
  setRepositoryId: (v: string) => void;
  setBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: null) => void;
  clearDraft: () => void;
  freshBranchEnabled: boolean;
  isLocalExecutor: boolean;
  /** Resolved on-disk path for the selected repository (workspace or discovered). Empty if not local. */
  repositoryLocalPath: string;
};
