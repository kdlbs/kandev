import type { ExecutorProfile, LocalRepository, Repository } from "@/lib/types/http";

// RepositorySelection mirrors the task-create dialog's two-tier model: a
// registered workspace repository (keyed by id) OR a filesystem-discovered
// repo not yet registered (keyed by local path). "none" represents an
// unfilled row (used only transiently — a row picks a real repository
// before it can be saved) or the single-picker's "Auto" fallback when the
// executor doesn't support multiple repositories.
export type RepositorySelection =
  | { kind: "none"; key?: string; branch?: string }
  | { kind: "registered"; id: string; key?: string; branch?: string }
  | {
      kind: "discovered";
      path: string;
      name: string;
      defaultBranch: string;
      key?: string;
      branch?: string;
    };

export const REPO_NONE_OPTION_ID = "__none__";
const DISCOVERED_PREFIX = "path:";

export function selectionToOptionId(sel: RepositorySelection): string {
  if (sel.kind === "registered") return sel.id;
  if (sel.kind === "discovered") return DISCOVERED_PREFIX + sel.path;
  return REPO_NONE_OPTION_ID;
}

// buildRepositoryItems lists registered + discovered repositories as picker
// options. `includeNone` adds the "None — no repository" sentinel option —
// used by the single-picker fallback (executor doesn't support multi-repo)
// but omitted from per-row multi-repo pickers, where removing a row is how
// you clear it.
//
// `t` is threaded in rather than imported from @/lib/i18n at module scope for
// two reasons: this is a plain .ts helper with no JSX, so the eslint guard
// never sees the option label at all, and both callers memoize the result —
// a captured module-level `t` would leave the memo stale after a locale
// switch. Every repository name in the list is user data and stays verbatim.
export function buildRepositoryItems(
  workspaceRepos: Repository[],
  discoveredRepos: LocalRepository[],
  t: (key: string) => string,
  options: { includeNone?: boolean } = {},
): Array<{ id: string; label: string }> {
  const registeredPaths = new Set(
    workspaceRepos
      .map((r) => r.local_path)
      .filter(Boolean)
      .map((p) => p.replace(/\/+$/, "")),
  );
  const items: Array<{ id: string; label: string }> =
    options.includeNone === false
      ? []
      : [{ id: REPO_NONE_OPTION_ID, label: t("automations:repositoryNone") }];
  for (const r of workspaceRepos) {
    items.push({ id: r.id, label: r.name || `${r.provider_owner}/${r.provider_name}` });
  }
  for (const r of discoveredRepos) {
    if (registeredPaths.has(r.path.replace(/\/+$/, ""))) continue;
    items.push({ id: DISCOVERED_PREFIX + r.path, label: `${r.name} - ${r.path}` });
  }
  return items;
}

export function pickSelectionFromOptionId(
  optionId: string,
  workspaceRepos: Repository[],
  discoveredRepos: LocalRepository[],
): RepositorySelection {
  if (optionId === REPO_NONE_OPTION_ID) return { kind: "none" };
  if (optionId.startsWith(DISCOVERED_PREFIX)) {
    const path = optionId.slice(DISCOVERED_PREFIX.length);
    const match = discoveredRepos.find((r) => r.path === path);
    // i18n-exempt: persisted repository name. See the comment below.
    return {
      kind: "discovered",
      path,
      // Persisted as the created repository's name (see resolveOneRepositoryId
      // in automation-payload.ts) — user data, so it stays English rather than
      // writing a locale-dependent name into the record.
      name: match?.name ?? path.split("/").pop() ?? "New Repository",
      defaultBranch: match?.default_branch ?? "",
    };
  }
  const reg = workspaceRepos.find((r) => r.id === optionId);
  return reg ? { kind: "registered", id: reg.id } : { kind: "none" };
}

export type ExecutorLike = {
  type: string;
  profiles?: Array<Pick<ExecutorProfile, "id" | "executor_type">>;
};

// resolveExecutorType looks up the executor type backing a given executor
// profile id — the same enrichment used by both the automation editor's
// picker (config-section.tsx) and its save-time normalization
// (automation-editor.tsx), so the two surfaces never disagree. Profiles
// returned by the executors list/boot payload don't always carry their own
// executor_type/executor_name (only the settings > Executors page's local
// mapper attaches those today), so this falls back to the parent executor's
// fields. The "local" system executor is excluded — mirrors
// task-create-dialog-computed.ts's identical enrichment/exclusion so this
// surface never disagrees with task creation either.
export function resolveExecutorType(
  executors: ExecutorLike[],
  executorProfileId: string,
): string | undefined {
  const profile = executors
    .filter((executor) => executor.type !== "local")
    .flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
      })),
    )
    .find((p) => p.id === executorProfileId);
  return profile?.executor_type;
}

// normalizeRepositorySelections enforces the single-repository invariant at
// the save boundary: the picker only accepts one row once the executor stops
// supporting multi-repo, but doesn't itself
// truncate the underlying selections array — a user who adds 2+ repos, then
// switches to an incompatible executor without touching the
// picker again, would otherwise still save every stale entry. Called right
// before resolveRepositoryIds so the persisted repository_ids always match
// what's actually rendered.
export function normalizeRepositorySelections(
  selections: RepositorySelection[],
  options: { supportsMultiRepo: boolean },
): RepositorySelection[] {
  if (options.supportsMultiRepo) return selections;
  return selections.slice(0, 1);
}
