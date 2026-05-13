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
  KanbanMultiState,
  WorkflowSnapshotData,
  WorkflowsState,
} from "@/lib/state/slices/kanban/types";
import type {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import type { useToast } from "@/components/toast-provider";

/**
 * One repository row in the task-create form. The form tracks every repo
 * (whether one or many) as an entry in `repositories[]`. There is no
 * "primary" — the backend treats them all equally and uses array order
 * for position. Removing the last row clears the form's repo selection.
 *
 * Exactly one of `repositoryId` (workspace repo) or `localPath` (on-machine
 * git folder discovered via the workspace's repo discovery) is set per row.
 * `key` is a stable client-side id used as React key and for
 * add/update/remove ops; it's not sent to the backend.
 */
export type TaskRepoRow = {
  key: string;
  /** Workspace repo id, when the user picked from the workspace's repos. */
  repositoryId?: string;
  /** On-machine repo path, when the user picked from discovered repos. */
  localPath?: string;
  branch: string;
};

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
  /** Display name of the currently selected executor profile (null if none). */
  selectedExecutorProfileName: string | null;
  /** True when an executor profile is selected and no agent profile is compatible with it. */
  noCompatibleAgent: boolean;
};

export type DialogComputedArgs = {
  fs: DialogFormState;
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  settingsData: {
    agentsLoaded: boolean;
    executorsLoaded: boolean;
    capabilitiesLoaded: boolean;
  };
  agentProfiles: AgentProfileOption[];
  workspaces: Workspace[];
  executors: Executor[];
  repositories: Repository[];
  workflows: Array<{ id: string; agent_profile_id?: string }>;
  snapshots: Record<string, WorkflowSnapshotData>;
};

export type TaskCreateEffectsArgs = {
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  repositories: Repository[];
  repositoriesLoading: boolean;
  agentProfiles: AgentProfileOption[];
  executors: Executor[];
  workspaceDefaults: Workspace | null | undefined;
  toast: ReturnType<typeof useToast>["toast"];
  workflows: Array<{ id: string; agent_profile_id?: string }>;
  /**
   * True when the currently-selected executor is the local-host one (no
   * worktree, no container). Drives the "reset row.branch on local switch"
   * effect so toggling worktree → local restores the chip default to the
   * workspace's current branch instead of leaving a stale pick from the
   * worktree run that would trigger a destructive `git checkout` on submit.
   */
  isLocalExecutor: boolean;
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
  /**
   * Unified list of repos on this task. Each row carries either a workspace
   * `repositoryId` or a discovered `localPath`, plus its base `branch`. The
   * order is the position the backend sees. There is no "primary" concept.
   */
  repositories: TaskRepoRow[];
  setRepositories: (v: TaskRepoRow[]) => void;
  addRepository: () => void;
  removeRepository: (key: string) => void;
  updateRepository: (key: string, patch: Partial<TaskRepoRow>) => void;
  /** GitHub URL mode: a separate flow that replaces the chip row with a URL input. */
  githubBranch: string;
  setGitHubBranch: (v: string) => void;
  agentProfileId: string;
  setAgentProfileId: (v: string) => void;
  executorId: string;
  setExecutorId: (v: string) => void;
  executorProfileId: string;
  setExecutorProfileId: (v: string) => void;
  discoveredRepositories: LocalRepository[];
  setDiscoveredRepositories: (v: LocalRepository[]) => void;
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
  /** True while resolving currentLocalBranch — distinguishes "still loading" from "no branch on disk" */
  currentLocalBranchLoading: boolean;
  setCurrentLocalBranchLoading: (v: boolean) => void;
  /** No-repo mode: when true the task is created with no repositories. */
  noRepository: boolean;
  setNoRepository: (v: boolean) => void;
  /** Optional host folder for repo-less tasks; empty means scratch workspace. */
  workspacePath: string;
  setWorkspacePath: (v: string) => void;
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
  /** Unified repo list from the form. Empty when in GitHub URL mode. */
  repositories: TaskRepoRow[];
  /** All on-machine discovered repos — used to look up `default_branch` for `localPath` rows. */
  discoveredRepositories: LocalRepository[];
  /** Workspace repositories — used to look up `default_branch` for `repositoryId` rows. */
  workspaceRepositories: Repository[];
  useGitHubUrl: boolean;
  githubUrl: string;
  githubPrHeadBranch: string | null;
  /** Branch for the GitHub URL flow. Per-row branches live on `repositories[i].branch`. */
  githubBranch: string;
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
  setRepositories: (v: TaskRepoRow[]) => void;
  setGitHubBranch: (v: string) => void;
  setAgentProfileId: (v: string) => void;
  setExecutorId: (v: string) => void;
  setSelectedWorkflowId: (v: string | null) => void;
  setFetchedSteps: (v: null) => void;
  clearDraft: () => void;
  freshBranchEnabled: boolean;
  isLocalExecutor: boolean;
  /** Resolved on-disk path for the selected repository (workspace or discovered). Empty if not local. */
  repositoryLocalPath: string;
  /** When true, the task is created with no repositories (repo-less mode). */
  noRepository: boolean;
  /** Optional host folder for repo-less tasks; empty means kandev creates a scratch workspace. */
  workspacePath: string;
  /**
   * Optional async transform applied to the trimmed description before the
   * API payload is built. Used by feature wrappers (e.g. Improve Kandev) to
   * append generated context like bundle file paths.
   */
  transformDescriptionBeforeSubmit?: (description: string) => Promise<string> | string;
};

import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import type { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";

/**
 * Props shared by all dialog-body variants (CreateModeBody, SessionModeBody,
 * DialogFormBody). Lives in this types module so task-create-dialog.tsx
 * stays under the per-file line cap — the dialog file is already a thin
 * orchestrator over many sibling modules and this is the largest type
 * surface left in it.
 */
export type DialogFormBodyProps = {
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  workspaceId: string | null;
  onJiraImport?: (ticket: JiraTicket) => void;
  onLinearImport?: (issue: LinearIssue) => void;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorProfileOptions: Array<{
    value: string;
    label: string;
    renderLabel?: () => React.ReactNode;
  }>;
  agentProfiles: AgentProfileOption[];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
  isCreatingSession: boolean;
  workflows: WorkflowsState["items"];
  snapshots: KanbanMultiState["snapshots"];
  effectiveWorkflowId: string | null;
  fs: DialogFormState;
  handleKeyDown: ReturnType<typeof useKeyboardShortcutHandler>;
  onTaskNameChange: (v: string) => void;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onAgentProfileChange: (v: string) => void;
  onExecutorProfileChange: (v: string) => void;
  onWorkflowChange: (v: string) => void;
  onToggleGitHubUrl?: () => void;
  onGitHubUrlChange: (v: string) => void;
  onToggleFreshBranch: (enabled: boolean) => void;
  onToggleNoRepository?: () => void;
  onWorkspacePathChange: (value: string) => void;
  enhance?: { onEnhance: () => void; isLoading: boolean; isConfigured: boolean };
  workflowAgentLocked: boolean;
  /** Workspace repositories — driven into the chip row for repo + branch picks. */
  repositories: Repository[];
  /** Computed in the parent: single-row + local executor + not URL mode. */
  freshBranchAvailable: boolean;
  /**
   * True when the selected executor profile runs locally on the host. Used
   * to lock the per-row branch pill (the user's checkout dictates the
   * branch for local execution; fresh-branch mode unlocks it).
   */
  isLocalExecutor: boolean;
  noCompatibleAgent: boolean;
  executorProfileName: string | null;
  /** Optional render slot above the description editor. */
  aboveDescriptionSlot?: React.ReactNode;
  /** Optional render slot inside the dialog body (rendered above the chip row). */
  extraFormSlot?: React.ReactNode;
  /** Optional render slot at the bottom of the dialog body (above the footer). */
  bottomSlot?: React.ReactNode;
  /** Optional override for the description placeholder. */
  descriptionPlaceholder?: string;
  /** When true, hides the workflow picker so the enforced workflow can't be swapped. */
  workflowLocked?: boolean;
};
