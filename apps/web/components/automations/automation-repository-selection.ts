import type { LocalRepository, Repository } from "@/lib/types/http";

// RepositorySelection mirrors the task-create dialog's two-tier model: a
// registered workspace repository (keyed by id) OR a filesystem-discovered
// repo not yet registered (keyed by local path). "none" represents an
// unfilled row (used only transiently — a row picks a real repository
// before it can be saved) or the single-picker's "Auto" fallback when the
// executor doesn't support multiple repositories.
export type RepositorySelection =
  | { kind: "none" }
  | { kind: "registered"; id: string }
  | { kind: "discovered"; path: string; name: string; defaultBranch: string };

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
export function buildRepositoryItems(
  workspaceRepos: Repository[],
  discoveredRepos: LocalRepository[],
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
      : [{ id: REPO_NONE_OPTION_ID, label: "None — no repository" }];
  for (const r of workspaceRepos) {
    items.push({ id: r.id, label: r.name || `${r.provider_owner}/${r.provider_name}` });
  }
  for (const r of discoveredRepos) {
    if (registeredPaths.has(r.path.replace(/\/+$/, ""))) continue;
    items.push({ id: DISCOVERED_PREFIX + r.path, label: `${r.name} — ${r.path}` });
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
    return {
      kind: "discovered",
      path,
      name: match?.name ?? path.split("/").pop() ?? "New Repository",
      defaultBranch: match?.default_branch ?? "",
    };
  }
  const reg = workspaceRepos.find((r) => r.id === optionId);
  return reg ? { kind: "registered", id: reg.id } : { kind: "none" };
}
