"use client";

import { useId } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
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
  const id = useId();
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{props.label}</Label>
      <p className="text-xs text-muted-foreground">{props.description}</p>
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger id={id} className="cursor-pointer">
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
  // forceRefresh: a repo created in settings doesn't update the shared store
  // slice, and the lazy fetch is gated by isLoaded — so without this the picker
  // could miss repositories created since the slice first loaded. The hook owns
  // the fetch (keeps data-flow in the hook layer, not the component).
  const { repositories } = useRepositories(workspaceId, !!workspaceId, true);
  const branchSource = repositoryId ? ({ kind: "id", workspaceId, repositoryId } as const) : null;
  const { branches, isLoading: branchesLoading } = useBranches(branchSource, !!repositoryId);
  // A branch name can appear twice (local + remote tracking, e.g. "main" and
  // origin/"main"), which would emit two <SelectItem value="main"> — Radix then
  // renders every matching item's text in the trigger ("mainmain") and React
  // warns on duplicate keys. Dedupe by name so each branch is one option.
  const uniqueBranchNames = Array.from(new Set(branches.map((b) => b.name)));
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
