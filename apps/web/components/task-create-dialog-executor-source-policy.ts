import type { ExecutorType } from "@/lib/types/http";
import { getWorkspaceSourceCapabilities } from "@/components/workspace-source-picker/executor-capabilities";

export type ExecutorSourceCounts = {
  sourceCount: number;
  repositoryCount: number;
  folderCount: number;
  localRepositoryCount?: number;
  remoteOriginRepositoryCount?: number;
};

export type ExecutorSourceMode = "scratch" | "folder" | "repository" | "mixed";

export type ExecutorSourcePolicy = {
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  mode: ExecutorSourceMode;
  folderAvailable: boolean;
  folderDisabledReason: "unknown_executor" | "requires_host_executor" | undefined;
  incompatible: boolean;
  incompatibleReason:
    | "folders_require_host_executor"
    | "repository_requires_remote_origin"
    | "repository_origin_unavailable"
    | undefined;
};

export function deriveExecutorSourcePolicy(args: {
  executorType: ExecutorType | string | null | undefined;
  counts: ExecutorSourceCounts;
}): ExecutorSourcePolicy {
  const capabilities = getWorkspaceSourceCapabilities(args.executorType);
  const { sourceCount, repositoryCount, folderCount } = args.counts;
  const localRepositoryCount = args.counts.localRepositoryCount ?? repositoryCount;
  const remoteOriginRepositoryCount = args.counts.remoteOriginRepositoryCount ?? 0;
  const mode = sourceMode(sourceCount, repositoryCount, folderCount);
  const folderDisabledReason = resolveFolderDisabledReason(capabilities);
  const foldersIncompatible = folderCount > 0 && !capabilities.canAddFolders;
  const repositoriesIncompatible =
    capabilities.requiresCloneableLocalRepository &&
    localRepositoryCount > remoteOriginRepositoryCount;
  const incompatible = foldersIncompatible || repositoriesIncompatible;
  return {
    capabilities,
    mode,
    folderAvailable: capabilities.canAddFolders,
    folderDisabledReason,
    incompatible,
    incompatibleReason: resolveIncompatibleReason(foldersIncompatible, repositoriesIncompatible),
  };
}

function sourceMode(
  sourceCount: number,
  repositoryCount: number,
  folderCount: number,
): ExecutorSourceMode {
  if (sourceCount === 0) return "scratch";
  if (folderCount > 0 && repositoryCount > 0) return "mixed";
  if (folderCount > 0) return "folder";
  return "repository";
}

function resolveFolderDisabledReason(
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>,
): ExecutorSourcePolicy["folderDisabledReason"] {
  if (!capabilities.executorCapabilitiesKnown) return "unknown_executor";
  if (!capabilities.canAddFolders) return "requires_host_executor";
  return undefined;
}

function resolveIncompatibleReason(
  foldersIncompatible: boolean,
  repositoriesIncompatible: boolean,
): ExecutorSourcePolicy["incompatibleReason"] {
  if (foldersIncompatible) return "folders_require_host_executor";
  if (repositoriesIncompatible) return "repository_requires_remote_origin";
  return undefined;
}

export function executorSourceIncompatibilityReasonKey(
  reason: ExecutorSourcePolicy["incompatibleReason"],
):
  | "task:foldersRequireHostExecutor"
  | "task:repositoryRequiresRemoteOrigin"
  | "task:noUsableRepositoryOrigin"
  | undefined {
  if (reason === "folders_require_host_executor") return "task:foldersRequireHostExecutor";
  if (reason === "repository_requires_remote_origin") {
    return "task:repositoryRequiresRemoteOrigin";
  }
  if (reason === "repository_origin_unavailable") return "task:noUsableRepositoryOrigin";
  return undefined;
}

export function executorSourcePolicyReasonKey(
  reason: ExecutorSourcePolicy["folderDisabledReason"],
): "task:addFolderExecutorUnknown" | "task:foldersRequireHostExecutor" | undefined {
  if (reason === "unknown_executor") return "task:addFolderExecutorUnknown";
  if (reason === "requires_host_executor") return "task:foldersRequireHostExecutor";
  return undefined;
}

export function shouldAutoSwitchFolderOnlyExecutor(args: {
  executorType: ExecutorType | string | null | undefined;
  counts: ExecutorSourceCounts;
  executorChoiceTouched: boolean;
}) {
  return (
    !args.executorChoiceTouched &&
    args.counts.sourceCount === 0 &&
    args.counts.folderCount === 0 &&
    args.counts.repositoryCount === 0 &&
    args.executorType === "worktree"
  );
}
