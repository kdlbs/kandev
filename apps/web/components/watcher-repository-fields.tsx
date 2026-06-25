"use client";

import { useEffect, useRef } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { listRepositories } from "@/lib/api";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import {
  NO_REPOSITORY,
  NO_REPOSITORY_LABEL,
  DEFAULT_BRANCH,
  DEFAULT_BRANCH_LABEL,
  branchPlaceholder,
  resolveBaseBranch,
  resolveRepositoryId,
} from "@/lib/watcher-repository-default";

type PickItem = { id: string; label: string };

function PickSelect(props: {
  label: string;
  description: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: PickItem[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      <p className="text-xs text-muted-foreground">{props.description}</p>
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

/**
 * Shared repository + base-branch picker for the Linear / Jira / Sentry watcher
 * dialogs. Binds the watcher to an optional repository so its tasks run against
 * that codebase instead of an empty scratch checkout; an empty repository =
 * unbound (repo-less task). How the repository is materialised (isolated
 * worktree vs in-place) is decided by the executor profile, not this field. The
 * base-branch select is disabled until a repository is chosen and defaults to
 * the repository's default branch. The `onChange` callbacks receive values
 * already collapsed from the dropdown sentinels back to "".
 */
export function WatcherRepositoryFields({
  workspaceId,
  repositoryId,
  baseBranch,
  onRepositoryChange,
  onBaseBranchChange,
}: {
  workspaceId: string;
  repositoryId: string;
  baseBranch: string;
  onRepositoryChange: (repositoryId: string) => void;
  onBaseBranchChange: (baseBranch: string) => void;
}) {
  const { repositories } = useRepositories(workspaceId, !!workspaceId);
  // useRepositories fetches a workspace's repos only once (gated by isLoaded),
  // and creating a repo in settings doesn't update that shared slice — so a
  // newly-created repo wouldn't appear here without a reload. Pull a fresh list
  // once each time the picker opens for a workspace so it always reflects
  // repositories created since the slice was first loaded.
  const setRepositories = useAppStore((s) => s.setRepositories);
  const refreshedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!workspaceId || refreshedFor.current === workspaceId) return;
    refreshedFor.current = workspaceId;
    listRepositories(workspaceId, undefined, { cache: "no-store" })
      .then((res) => setRepositories(workspaceId, res.repositories))
      .catch(() => {
        // Leave the cached list in place on failure; useRepositories still
        // serves whatever was previously loaded.
      });
  }, [workspaceId, setRepositories]);
  const branchSource = repositoryId ? ({ kind: "id", workspaceId, repositoryId } as const) : null;
  const { branches, isLoading: branchesLoading } = useBranches(branchSource, !!repositoryId);
  // A branch name can appear twice (local + remote tracking, e.g. "main" and
  // origin/"main"), which would emit two <SelectItem value="main"> — Radix then
  // renders every matching item's text in the trigger ("mainmain") and React
  // warns on duplicate keys. Dedupe by name so each branch is one option.
  const uniqueBranchNames = Array.from(new Set(branches.map((b) => b.name)));
  return (
    <div className="grid grid-cols-2 gap-4">
      <PickSelect
        label="Repository"
        description="Optional — the repository the agent works in."
        value={repositoryId || NO_REPOSITORY}
        onChange={(v) => onRepositoryChange(resolveRepositoryId(v))}
        placeholder={NO_REPOSITORY_LABEL}
        items={[
          { id: NO_REPOSITORY, label: NO_REPOSITORY_LABEL },
          ...repositories.map((r) => ({ id: r.id, label: r.name })),
        ]}
      />
      <PickSelect
        label="Base Branch"
        description="The base branch the agent starts from."
        value={baseBranch || DEFAULT_BRANCH}
        onChange={(v) => onBaseBranchChange(resolveBaseBranch(v))}
        placeholder={branchPlaceholder(repositoryId, branchesLoading)}
        items={[
          { id: DEFAULT_BRANCH, label: DEFAULT_BRANCH_LABEL },
          ...uniqueBranchNames.map((name) => ({ id: name, label: name })),
        ]}
        disabled={!repositoryId || branchesLoading}
      />
    </div>
  );
}
