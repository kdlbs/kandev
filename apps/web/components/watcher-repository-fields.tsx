"use client";

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
 * dialogs. Binds the watcher to an optional repository so its tasks launch in
 * an isolated worktree; an empty repository = unbound (repo-less task). The
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
  const branchSource = repositoryId
    ? ({ kind: "id", workspaceId, repositoryId } as const)
    : null;
  const { branches, isLoading: branchesLoading } = useBranches(branchSource, !!repositoryId);
  return (
    <div className="grid grid-cols-2 gap-4">
      <PickSelect
        label="Repository"
        description="Optional — run tasks in an isolated worktree of this repo."
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
        description="Branch the per-task worktree is cut from."
        value={baseBranch || DEFAULT_BRANCH}
        onChange={(v) => onBaseBranchChange(resolveBaseBranch(v))}
        placeholder={branchPlaceholder(repositoryId, branchesLoading)}
        items={[
          { id: DEFAULT_BRANCH, label: DEFAULT_BRANCH_LABEL },
          ...branches.map((b) => ({ id: b.name, label: b.name })),
        ]}
        disabled={!repositoryId || branchesLoading}
      />
    </div>
  );
}
