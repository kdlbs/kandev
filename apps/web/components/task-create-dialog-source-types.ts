import type { Branch, Repository, RepositorySet } from "@/lib/types/http";
import type { RepositoryCheckoutOptions } from "@/lib/types/repository-checkout-options";

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
  /** Explicit effective base copied from a repository set or chosen in the form. */
  baseBranch?: string;
  /** Saved repository policy selected for this row. */
  branchPolicyId?: string;
  /** Explicit remote-origin checkout mode for a remote executor. */
  checkoutSource?: "remote_origin";
  /** Credential-free origin identity confirmed by the backend. */
  expectedOrigin?: string;
  /** Branches read from the verified origin while remote clone mode is active. */
  remoteBranches?: Branch[];
};

/** A workspace or host-local repository in the ordered task draft. */
export type TaskLocalRepositorySelection = TaskRepoRow & { kind: "local" };

/** Repository fields needed to rehydrate an edit form without losing a policy snapshot. */
export type TaskRepositorySnapshot = {
  repository_id: string;
  base_branch?: string;
  id?: string;
  task_id?: string;
  checkout_branch?: string;
  branch_policy_id?: string;
  branch_policy_name?: string;
  branch_policy_base_branch?: string;
  branch_policy_branch_template?: string;
  branch_policy_pull_request_target?: string;
  position?: number;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

/**
 * One remote-repo row in the task-create form. Each row is either a
 * picker-selected repo or a manually-pasted URL/PR — both collapse to a
 * single `url` field, with `source` only used for UI affordance.
 */
export type TaskRemoteRepoRow = {
  checkoutOptions?: RepositoryCheckoutOptions;
  key: string; // stable client-side React key
  url: string; // canonical https://… or paste-as-typed
  /** Exact credential-free clone URL returned by provider URL inspection. */
  remoteUrl?: string;
  branch: string;
  source: "picker" | "paste";
  prNumber?: number;
  prBaseBranch?: string;
  prHeadBranch?: string;
  // Optional metadata when source === "picker":
  provider?: string;
  providerHost?: string;
  providerScope?: string;
  providerRepoId?: string;
  providerOwner?: string;
  providerName?: string;
  fullName?: string; // "owner/name"
};

/** A provider-backed or pasted repository in the ordered task draft. */
export type TaskRemoteRepositorySelection = TaskRemoteRepoRow & { kind: "remote" };

/** A live host folder in the ordered task workspace draft. */
export type TaskWorkspaceFolderSelection = {
  kind: "folder";
  key: string;
  localPath: string;
  displayName?: string;
};

/** Every workspace source attached to a task, in the order submitted. */
export type TaskWorkspaceSelection =
  | TaskLocalRepositorySelection
  | TaskRemoteRepositorySelection
  | TaskWorkspaceFolderSelection;

/** @deprecated Use TaskWorkspaceSelection for new source-aware callers. */
export type TaskRepositorySelection = TaskWorkspaceSelection;

export type TaskRepositorySetsConfig = {
  sets: RepositorySet[];
  onApply: (set: RepositorySet) => void;
  rows?: TaskRepoRow[];
  repositories?: Repository[];
  save?: {
    workspaceId: string;
    rows: TaskRepoRow[];
    repositories: Repository[];
    isLocalExecutor: boolean;
    freshBranchEnabled: boolean;
    selections?: TaskRepositorySelection[];
    open: boolean;
    setOpen: (open: boolean) => void;
  } | null;
};
