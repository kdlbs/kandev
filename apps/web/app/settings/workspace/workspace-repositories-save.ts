import {
  createRepositoryAction,
  createRepositoryScriptAction,
  deleteRepositoryScriptAction,
  updateRepositoryAction,
  updateRepositoryScriptAction,
} from "@/app/actions/workspaces";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";
import type { RepositoryWithScripts } from "./workspace-repositories-dirty";

type ScriptWrite = {
  id: string;
  run: () => Promise<RepositoryScript | void>;
};

function scriptChanged(script: RepositoryScript, saved: RepositoryScript | undefined) {
  return (
    !saved ||
    script.name !== saved.name ||
    script.command !== saved.command ||
    script.position !== saved.position
  );
}

function scriptWrites(
  repo: RepositoryWithScripts,
  savedScripts: RepositoryScript[],
): ScriptWrite[] {
  const currentIds = new Set(repo.scripts.map((script) => script.id));
  const writes: ScriptWrite[] = savedScripts
    .filter((script) => !currentIds.has(script.id))
    .map((script) => ({
      id: script.id,
      run: () => deleteRepositoryScriptAction(script.id),
    }));
  repo.scripts.forEach((script, index) => {
    if (
      !scriptChanged(
        script,
        savedScripts.find((saved) => saved.id === script.id),
      )
    )
      return;
    writes.push({
      id: script.id,
      run: () =>
        script.id.startsWith("temp-script-")
          ? createRepositoryScriptAction({
              repository_id: repo.id,
              // i18n-exempt: persisted script name, sent to a shell verbatim.
              name: script.name.trim() || "New Script",
              command: script.command.trim() || 'echo ""',
              position: script.position ?? index,
            })
          : updateRepositoryScriptAction(script.id, {
              name: script.name,
              command: script.command,
              position: script.position ?? index,
            }),
    });
  });
  return writes;
}

function repositoryPayload(repo: RepositoryWithScripts) {
  return {
    name: repo.name,
    source_type: repo.source_type,
    local_path: repo.local_path,
    provider: repo.provider,
    provider_repo_id: repo.provider_repo_id,
    provider_host: repo.provider_host,
    provider_owner: repo.provider_owner,
    provider_name: repo.provider_name,
    default_branch: repo.default_branch,
    worktree_branch_prefix: repo.worktree_branch_prefix,
    worktree_branch_template: repo.worktree_branch_template ?? defaultWorktreeBranchTemplate,
    pull_before_worktree: repo.pull_before_worktree,
    setup_script: repo.setup_script,
    cleanup_script: repo.cleanup_script,
    dev_script: repo.dev_script,
    copy_files: repo.copy_files,
    secret_bindings: repo.secret_bindings ?? [],
  };
}

async function saveRepositoryMetadata(
  repo: RepositoryWithScripts,
  workspaceId: string,
): Promise<Repository> {
  const payload = repositoryPayload(repo);
  if (!repo.id.startsWith("temp-repo-")) return updateRepositoryAction(repo.id, payload);
  return createRepositoryAction({
    ...payload,
    workspace_id: workspaceId,
    // i18n-exempt: persisted repository name, independent of the UI locale.
    name: repo.name.trim() || "New Repository",
    source_type: repo.source_type || "local",
  });
}

/** Publish acknowledged persistence separately from the complete submitted draft. */
export async function saveRepositoryDraft(
  submitted: RepositoryWithScripts,
  saved: RepositoryWithScripts | undefined,
  workspaceId: string,
  publish: (baseline: RepositoryWithScripts, draft: RepositoryWithScripts) => void,
): Promise<void> {
  const updated = await saveRepositoryMetadata(submitted, workspaceId);
  const savedScripts = saved?.scripts ?? [];
  const writes = scriptWrites({ ...submitted, id: updated.id }, savedScripts);
  // A rejected sibling must not release Save while an admitted write is still pending.
  const results = await Promise.allSettled(writes.map((write) => write.run()));
  const accepted = new Map(savedScripts.map((script) => [script.id, script]));
  const acknowledgements = new Map<string, RepositoryScript>();
  results.forEach((result, index) => {
    if (result.status !== "fulfilled") return;
    const originalId = writes[index].id;
    accepted.delete(originalId);
    if (result.value) {
      accepted.set(result.value.id, result.value);
      acknowledgements.set(originalId, result.value);
    }
  });
  const baseline = {
    ...updated,
    scripts: [...accepted.values()].sort((a, b) => a.position - b.position),
  };
  const draft = {
    ...updated,
    scripts: submitted.scripts.map(
      (script) => acknowledgements.get(script.id) ?? { ...script, repository_id: updated.id },
    ),
  };
  publish(baseline, draft);
  const failure = results.find((result) => result.status === "rejected");
  if (failure?.status === "rejected") throw failure.reason;
}
