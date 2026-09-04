import { Badge } from "@kandev/ui/badge";

import { t } from "@/lib/i18n";
import type { Branch } from "@/lib/types/http";
import type { BranchOption } from "./branch-selector";

// Conventional default branches surfaced at the top of the dropdown when no
// search term is active. cmdk preserves option order on empty queries, so a
// stable sort here lifts main/master/develop above feature branches.
const PREFERRED_BRANCH_NAMES = ["main", "master", "develop"];

function branchPriority(b: Branch): number {
  const idx = PREFERRED_BRANCH_NAMES.indexOf(b.name);
  if (idx === -1) return PREFERRED_BRANCH_NAMES.length;
  return idx;
}

export function sortBranches(branches: Branch[]): Branch[] {
  return [...branches].sort((a, b) => {
    const pa = branchPriority(a);
    const pb = branchPriority(b);
    if (pa !== pb) return pa - pb;
    // Within the same priority bucket, locals before remotes. This matches
    // the auto-select preference (`main` over `origin/main`).
    if (a.type !== b.type) return a.type === "local" ? -1 : 1;
    return 0;
  });
}

const BRANCH_SEGMENT_RE = /[/_.\-\s]+/;

export function buildBranchKeywords(name: string, remote?: string): string[] {
  const out = new Set<string>();
  out.add(name);
  const leafIdx = name.lastIndexOf("/");
  if (leafIdx >= 0) out.add(name.slice(leafIdx + 1));
  for (const seg of name.split(BRANCH_SEGMENT_RE)) {
    if (seg) out.add(seg);
  }
  if (remote) out.add(remote);
  return Array.from(out);
}

export function branchToOption(b: Branch): BranchOption {
  // Remote branches keep their "origin/" prefix so they're distinguishable
  // from local branches with the same short name (e.g. "main" vs "origin/main").
  // Without the prefix, the dropdown shows two indistinguishable rows.
  const display = b.type === "remote" && b.remote ? `${b.remote}/${b.name}` : b.name;
  // `||` (not `??`) so an empty-string `remote` falls back too. Provider-backed
  // workspace repos (URL-added) list branches without a tracking remote, so
  // the backend sends `remote: ""`; `??` would render an invisible empty badge.
  const badge = b.type === "local" ? "local" : b.remote || "remote";
  return {
    value: display,
    label: display,
    keywords: buildBranchKeywords(b.name, b.remote),
    group: "branches",
    groupLabel: t("task:branchesGroup"),
    renderLabel: () => (
      <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
        <span className="truncate" title={display}>
          {display}
        </span>
        <Badge variant="outline" className="text-xs shrink-0">
          {badge}
        </Badge>
      </span>
    ),
  };
}

export function computeBranchPlaceholder(
  hasRepo: boolean,
  loading: boolean,
  optionCount: number,
): string {
  if (!hasRepo) return "branch";
  if (loading) return "loading…";
  if (optionCount === 0) return t("task:noBranchesShort");
  return "branch";
}
