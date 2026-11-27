import type { LocalRepository, Repository } from "@/lib/types/http";
import type {
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskWorkspaceSelection,
} from "@/components/task-create-dialog-types";
import type { UsePRInfoByURLResult } from "@/hooks/domains/github/use-pr-info-by-url";
import { splitLocalExecutorBranches } from "@/components/task-create-dialog-branch-utils";
import type { WorkspaceSourceRequest } from "@/lib/types/http-workspace-sources";
import type { CreateTaskParams } from "@/components/task-create-dialog-helpers";

type CreateTaskRepositoryPayload = NonNullable<CreateTaskParams["repositories"]>[number];
type RemoteRepoPRMetadata = {
  headBranch?: string;
  baseBranch?: string;
  number?: number;
};
type FreshRepositoryPayload = Partial<{
  fresh_branch: boolean;
  new_branch_name: string;
  confirm_discard: boolean;
  consented_dirty_files: string[];
}>;

export function buildRepositoriesPayload(opts: {
  /** Ordered mixed rows from the current task draft. */
  selections?: TaskWorkspaceSelection[];
  /** True when the form is in GitHub Remote (URL) mode. */
  useRemote: boolean;
  /** Remote-URL rows; non-empty `url` rows are mapped 1:1 to payload entries. */
  remoteRepos: TaskRemoteRepoRow[];
  /**
   * Per-URL PR-info cache. Consulted for each remote row whose URL is a PR
   * URL: if the row's branch equals the PR head (auto-fill or user-confirmed
   * default), the payload anchors `base_branch` to the PR's actual target
   * from the API so origin can resolve it even when the head only lives on
   * a fork. Optional — non-Remote call sites can omit it.
   */
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">;
  repositories: TaskRepoRow[];
  /** Used to look up `default_branch` for `localPath` rows. */
  discoveredRepositories: LocalRepository[];
  /** Workspace repositories — used to look up `default_branch` for `repositoryId` rows. */
  workspaceRepositories?: Repository[];
  /**
   * For the local executor (no worktree), the chip's branch field represents
   * the working branch on disk, not the parent integration branch. Send it as
   * `checkout_branch` so the session's `base_branch` stays anchored to the
   * repo's `default_branch` (which is what git log / cumulative diff use as
   * the merge-base reference). Without this, "new branch on local executor"
   * collapses the merge-base recomputation to HEAD and the changes panel
   * goes empty after a refresh.
   */
  isLocalExecutor?: boolean;
  /** True when the selected executor will materialize local repositories from their origin. */
  remoteOriginMode?: boolean;
  /**
   * Optional fresh-branch metadata. The UI gates this to single-row + local
   * executor; when present we apply it to every row (which is at most one).
   */
  freshBranch?: { confirmDiscard: boolean; consentedDirtyFiles: string[] };
}): NonNullable<CreateTaskParams["repositories"]> {
  if (opts.selections) {
    return buildMixedRepositoryPayload(opts.selections, opts);
  }
  if (opts.useRemote) {
    return buildRemoteRepoPayload(opts);
  }
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  // Fresh-branch flow inverts the chip's semantics: instead of "the working
  // branch I'm on", row.branch becomes "the base I want to fork from". The
  // backend then creates a new branch and rewrites repos[i].BaseBranch to it.
  // Splitting here would force `base_branch=default_branch, checkout_branch=
  // <picked-base>` and the backend would fork from the wrong base. Skip the
  // split entirely when fresh-branch is active.
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return opts.repositories
    .filter((row) => row.repositoryId || row.localPath)
    .map((row) => buildLocalRepositoryPayloadRow(row, opts, fresh, isLocalExecutor));
}

function buildMixedRepositoryPayload(
  selections: TaskWorkspaceSelection[],
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): NonNullable<CreateTaskParams["repositories"]> {
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return selections.flatMap((selection) => {
    if (selection.kind === "folder") return [];
    if (selection.kind === "remote") {
      return selection.url.trim() ? [buildRemoteRepoPayloadRow(selection, opts.prInfoByUrl)] : [];
    }
    if (!selection.repositoryId && !selection.localPath) return [];
    return [buildLocalRepositoryPayloadRow(selection, opts, fresh, isLocalExecutor)];
  });
}

/** Builds the ordered, tagged source payload used by the new task-creation contract. */
export function buildWorkspaceSourcesPayload(
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): WorkspaceSourceRequest[] {
  const selections =
    opts.selections ?? resolveLegacySelections(opts.repositories, opts.remoteRepos);
  const fresh = opts.freshBranch
    ? {
        fresh_branch: true,
        confirm_discard: opts.freshBranch.confirmDiscard,
        consented_dirty_files: opts.freshBranch.consentedDirtyFiles,
      }
    : {};
  const isLocalExecutor = !!opts.isLocalExecutor && !opts.freshBranch;
  return selections.flatMap((selection): WorkspaceSourceRequest[] => {
    if (selection.kind === "folder") {
      const localPath = selection.localPath.trim();
      if (!localPath) return [];
      return [
        {
          kind: "folder" as const,
          local_path: localPath,
          ...(selection.displayName ? { display_name: selection.displayName } : {}),
        },
      ];
    }
    if (selection.kind === "remote") {
      if (!selection.url.trim()) return [];
      const repository = buildRemoteRepoPayloadRow(selection, opts.prInfoByUrl);
      return [workspaceRepositorySource(repository)];
    }
    if (!selection.repositoryId && !selection.localPath) return [];
    const repository = buildLocalRepositoryPayloadRow(selection, opts, fresh, isLocalExecutor);
    return [workspaceRepositorySource(repository)];
  });
}

function workspaceRepositorySource(
  repository: CreateTaskRepositoryPayload,
): WorkspaceSourceRequest & { kind: "repository" } {
  return {
    kind: "repository",
    ...(repository.checkout_options ? { checkout_options: repository.checkout_options } : {}),
    ...repositoryIdentityFields(repository),
    ...repositoryBranchFields(repository),
    ...repositoryFreshBranchFields(repository),
  };
}

type RepositorySourceFields = Partial<
  Omit<Extract<WorkspaceSourceRequest, { kind: "repository" }>, "kind">
>;

function repositoryIdentityFields(repository: CreateTaskRepositoryPayload): RepositorySourceFields {
  return {
    ...(repository.repository_id ? { repository_id: repository.repository_id } : {}),
    ...(repository.local_path ? { local_path: repository.local_path } : {}),
    ...(repository.github_url ? { github_url: repository.github_url } : {}),
    ...(repository.remote_url ? { remote_url: repository.remote_url } : {}),
    ...(repository.provider ? { provider: repository.provider } : {}),
    ...(repository.provider_host ? { provider_host: repository.provider_host } : {}),
    ...(repository.provider_scope ? { provider_scope: repository.provider_scope } : {}),
    ...(repository.provider_repo_id ? { provider_repo_id: repository.provider_repo_id } : {}),
    ...(repository.provider_owner ? { provider_owner: repository.provider_owner } : {}),
    ...(repository.provider_name ? { provider_name: repository.provider_name } : {}),
    ...(repository.checkout_source ? { checkout_source: repository.checkout_source } : {}),
    ...(repository.expected_origin ? { expected_origin: repository.expected_origin } : {}),
  };
}

function repositoryBranchFields(repository: CreateTaskRepositoryPayload): RepositorySourceFields {
  return {
    ...(repository.base_branch ? { base_branch: repository.base_branch } : {}),
    ...(repository.checkout_branch ? { checkout_branch: repository.checkout_branch } : {}),
    ...(repository.branch_policy_id ? { branch_policy_id: repository.branch_policy_id } : {}),
    ...(repository.pr_number ? { pr_number: repository.pr_number } : {}),
  };
}

function repositoryFreshBranchFields(
  repository: CreateTaskRepositoryPayload,
): RepositorySourceFields {
  return {
    ...(repository.fresh_branch ? { fresh_branch: repository.fresh_branch } : {}),
    ...(repository.new_branch_name ? { new_branch_name: repository.new_branch_name } : {}),
    ...(repository.confirm_discard !== undefined
      ? { confirm_discard: repository.confirm_discard }
      : {}),
    ...(repository.consented_dirty_files
      ? { consented_dirty_files: repository.consented_dirty_files }
      : {}),
  };
}

function buildLocalRepositoryPayloadRow(
  row: TaskRepoRow,
  opts: Parameters<typeof buildRepositoriesPayload>[0],
  fresh: FreshRepositoryPayload,
  isLocalExecutor: boolean,
): CreateTaskRepositoryPayload {
  const defaultBranch = resolveRowDefaultBranch(row, opts);
  const branches = opts.freshBranch
    ? { base_branch: row.branch || undefined, checkout_branch: undefined }
    : splitLocalExecutorBranches({
        // Restored local sources can carry both the branch checked out on the
        // host and the saved integration base. Worktree-backed executors use the
        // base while direct local execution keeps the checkout branch.
        rowBranch: isLocalExecutor ? row.branch : row.baseBranch || row.branch,
        defaultBranch,
        baseBranch: row.baseBranch,
        isLocalExecutor,
      });
  if (row.repositoryId) {
    return {
      repository_id: row.repositoryId,
      ...(row.branchPolicyId ? { branch_policy_id: row.branchPolicyId } : {}),
      base_branch: branches.base_branch,
      checkout_branch: branches.checkout_branch,
      ...localRepositoryOriginFields(row, opts),
      ...fresh,
    };
  }
  return {
    repository_id: "",
    base_branch: branches.base_branch,
    checkout_branch: branches.checkout_branch,
    local_path: row.localPath,
    default_branch: defaultBranch || undefined,
    ...localRepositoryOriginFields(row, opts),
    ...fresh,
  };
}

function localRepositoryOriginFields(
  row: TaskRepoRow,
  opts: Parameters<typeof buildRepositoriesPayload>[0],
): Pick<CreateTaskRepositoryPayload, "checkout_source" | "expected_origin"> {
  if (opts.remoteOriginMode === false) return {};
  return {
    ...(row.checkoutSource ? { checkout_source: row.checkoutSource } : {}),
    ...(row.expectedOrigin ? { expected_origin: row.expectedOrigin } : {}),
  };
}

/**
 * Builds the `repos: [{ github_url, branch }]` payload from the remote-URL
 * rows. Rows with an empty URL are dropped silently — they're partially
 * filled rows the user hasn't completed yet.
 *
 * Per-row PR-info inference: if a row's URL is a PR URL and the row's
 * branch equals the PR's head branch (auto-selected by the chip or
 * user-confirmed via "leave default"), the payload anchors `base_branch`
 * to the PR's actual target from the GitHub API and surfaces the PR head
 * as `checkout_branch`. This keeps fork PRs resolvable on `origin` (their
 * head doesn't live there, but the base does). When the user overrides
 * the branch to something other than the PR head, we treat their pick as
 * the base and drop `checkout_branch`.
 */
function buildRemoteRepoPayload(opts: {
  remoteRepos: TaskRemoteRepoRow[];
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">;
}): NonNullable<CreateTaskParams["repositories"]> {
  const nonEmpty = opts.remoteRepos.filter((r) => r.url.trim() !== "");
  if (nonEmpty.length === 0) return [];
  return nonEmpty.map((row) => buildRemoteRepoPayloadRow(row, opts.prInfoByUrl));
}

function buildRemoteRepoPayloadRow(
  row: TaskRemoteRepoRow,
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">,
): CreateTaskRepositoryPayload {
  const url = row.url.trim();
  const metadata = remoteRepoPRMetadata(row, url, prInfoByUrl);
  const payload = metadata
    ? buildRemoteRepoPRPayload(row, url, metadata)
    : buildPlainRemoteRepoPayload(row, url);
  return row.checkoutOptions ? { ...payload, checkout_options: row.checkoutOptions } : payload;
}

function remoteRepoPRMetadata(
  row: TaskRemoteRepoRow,
  url: string,
  prInfoByUrl?: Pick<UsePRInfoByURLResult, "info">,
): RemoteRepoPRMetadata | null {
  // The cache is keyed on the trimmed URL (ensure() also trims), so we
  // must look it up with the trimmed value too. Passing `row.url` directly
  // would miss the cache when the user has stray whitespace around their
  // URL and silently lose the PR base-branch anchoring.
  const prInfo = prInfoByUrl?.info(url);
  const number = prInfo?.prNumber ?? row.prNumber;
  if (!prInfo && !number) return null;
  return {
    headBranch: prInfo?.prHeadBranch ?? row.prHeadBranch,
    baseBranch: prInfo?.prBaseBranch ?? row.prBaseBranch,
    number,
  };
}

function buildRemoteRepoPRPayload(
  row: TaskRemoteRepoRow,
  url: string,
  metadata: RemoteRepoPRMetadata,
): CreateTaskRepositoryPayload {
  const isPrAutoSelection = !!metadata.headBranch && row.branch === metadata.headBranch;
  const baseBranch = isPrAutoSelection ? metadata.baseBranch || undefined : row.branch || undefined;
  return {
    repository_id: "",
    base_branch: baseBranch,
    checkout_branch: isPrAutoSelection ? metadata.headBranch || undefined : undefined,
    pr_number: isPrAutoSelection ? metadata.number || undefined : undefined,
    ...remoteRepositoryLocator(row, url),
  };
}

function buildPlainRemoteRepoPayload(
  row: TaskRemoteRepoRow,
  url: string,
): CreateTaskRepositoryPayload {
  return {
    repository_id: "",
    base_branch: row.branch || undefined,
    checkout_branch: undefined,
    ...remoteRepositoryLocator(row, url),
  };
}

function remoteRepositoryLocator(
  row: TaskRemoteRepoRow,
  url: string,
): Pick<
  CreateTaskRepositoryPayload,
  | "github_url"
  | "remote_url"
  | "provider"
  | "provider_host"
  | "provider_scope"
  | "provider_repo_id"
  | "provider_owner"
  | "provider_name"
> {
  if (!row.provider || row.provider === "github") {
    return { github_url: url };
  }
  return {
    remote_url: row.remoteUrl?.trim() || url,
    provider: row.provider,
    provider_host: row.providerHost,
    provider_scope: row.providerScope,
    provider_repo_id: row.providerRepoId,
    provider_owner: row.providerOwner,
    provider_name: row.providerName,
  };
}

function resolveRowDefaultBranch(
  row: TaskRepoRow,
  opts: {
    discoveredRepositories: LocalRepository[];
    workspaceRepositories?: Repository[];
  },
): string | undefined {
  if (row.repositoryId) {
    return opts.workspaceRepositories?.find((r) => r.id === row.repositoryId)?.default_branch;
  }
  return opts.discoveredRepositories.find((d) => d.path === row.localPath)?.default_branch;
}

/**
 * For the local executor: return `base_branch=defaultBranch`,
 * `checkout_branch=rowBranch` so the session anchors merge-base to the repo's
 * integration branch while the preparer still checks out the user's working
 * branch. When `rowBranch === defaultBranch` we omit checkout_branch — the
 * preparer treats matching values as "use current state" and skips git ops.
 *
 * For non-local executors (worktree-based): keep the historical shape where
 * `base_branch=rowBranch` (the worktree creates a new branch off of it).
 */

export function resolveLegacySelections(
  repositories: TaskRepoRow[],
  remoteRepos: TaskRemoteRepoRow[],
): TaskWorkspaceSelection[] {
  return [
    ...repositories.map((row) => ({ kind: "local" as const, ...row })),
    ...remoteRepos.map((row) => ({ kind: "remote" as const, ...row })),
  ];
}
