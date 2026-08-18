import type { PluginRepositoryProviderRegistration } from "./registry";
import type { PluginHostRepository } from "./types";
import { t } from "@/lib/i18n";

type TaskRepositoryLink = { repository_id: string; position?: number };
type CreationTask = {
  id: string;
  workspaceId?: string;
  workspace_id?: string;
  repositoryId?: string | null;
  repositories?: readonly TaskRepositoryLink[];
};
type CreationRepository = Pick<PluginHostRepository, "id" | "name" | "provider"> &
  Partial<Pick<PluginHostRepository, "workspace_id">>;

export type ChangeRequestProviderTarget = {
  provider: PluginRepositoryProviderRegistration;
  workspaceId: string;
  taskId: string;
  repositoryId: string;
  repository: PluginHostRepository;
};

function taskRepositoryLinks(task: CreationTask): readonly TaskRepositoryLink[] {
  if (task.repositories?.length) {
    return [...task.repositories].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  }
  if (task.repositoryId) return [{ repository_id: task.repositoryId, position: 0 }];
  return [];
}

export function resolveChangeRequestProviderTarget({
  task,
  repositories,
  repositoryScope,
  getProvider,
}: {
  task: CreationTask | undefined;
  repositories: readonly CreationRepository[];
  repositoryScope?: string;
  getProvider(id: string): PluginRepositoryProviderRegistration | undefined;
}): ChangeRequestProviderTarget | null {
  if (!task) return null;
  const workspaceId = task.workspaceId ?? task.workspace_id;
  if (!workspaceId) return null;
  const links = taskRepositoryLinks(task);
  const linkedIds = new Set(links.map((link) => link.repository_id));
  const linked = repositories.filter(
    (repository) =>
      linkedIds.has(repository.id) &&
      (!repository.workspace_id || repository.workspace_id === workspaceId),
  );
  const matches =
    repositoryScope === undefined || repositoryScope === ""
      ? linked.filter((repository) => repository.id === links[0]?.repository_id)
      : linked.filter((repository) => repository.name === repositoryScope);
  if (matches.length !== 1) return null;
  const repository = matches[0];
  const provider = getProvider(repository.provider);
  if (!provider?.createChangeRequest) return null;
  return {
    provider,
    workspaceId,
    taskId: task.id,
    repositoryId: repository.id,
    repository: {
      ...repository,
      workspace_id: repository.workspace_id ?? workspaceId,
    },
  };
}

type PushResult = { success: boolean; output?: string; error?: string };
type NativeCreateResult = {
  success: boolean;
  branch_pushed: boolean;
  pr_url?: string;
  provider?: string;
  output?: string;
  error?: string;
  linked?: boolean;
  association_error?: string;
};

export async function createChangeRequestWithProvider({
  target,
  push,
  repositoryScope,
  title,
  body,
  baseBranch,
  draft,
  branchAlreadyPushed,
  sessionId,
  signal,
}: {
  target: ChangeRequestProviderTarget;
  push(options: { setUpstream: boolean }, repositoryScope?: string): Promise<PushResult>;
  repositoryScope?: string;
  title: string;
  body: string;
  baseBranch?: string;
  draft: boolean;
  branchAlreadyPushed: boolean;
  sessionId: string;
  signal: AbortSignal;
}): Promise<NativeCreateResult> {
  let pushOutput = "";
  if (!branchAlreadyPushed) {
    const pushed = await push({ setUpstream: true }, repositoryScope);
    pushOutput = pushed.output ?? "";
    if (!pushed.success) {
      return {
        success: false,
        branch_pushed: false,
        output: pushOutput,
        ...(pushed.error ? { error: pushed.error } : {}),
      };
    }
  }
  try {
    const created = await target.provider.createChangeRequest!({
      workspaceId: target.workspaceId,
      taskId: target.taskId,
      sessionId,
      repositoryId: target.repositoryId,
      repository: target.repository,
      title,
      body,
      ...(baseBranch ? { baseBranch } : {}),
      draft: target.provider.supportsDraft === false ? false : draft,
      signal,
    });
    return {
      success: true,
      branch_pushed: true,
      pr_url: created.url,
      provider: created.provider ?? target.provider.id,
      output: created.output ?? pushOutput,
      ...(created.linked === undefined ? {} : { linked: created.linked }),
      ...(created.associationError ? { association_error: created.associationError } : {}),
    };
  } catch (error) {
    return {
      success: false,
      branch_pushed: true,
      output: pushOutput,
      error: error instanceof Error ? error.message : t("integrations:changeRequestCreationFailed"),
    };
  }
}
