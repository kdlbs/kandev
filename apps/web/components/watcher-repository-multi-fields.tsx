"use client";

import { useState } from "react";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { DEFAULT_BRANCH, resolveBaseBranch } from "@/lib/watcher-repository-default";

/** One repository binding of a watcher: the repo plus the branch its per-task
 * worktree is cut from ("" = the repo's default branch, resolved at save). */
export type WatcherRepoBinding = {
  repositoryId: string;
  baseBranch: string;
};

/**
 * Multi-repository picker for the watcher dialogs: select/add several
 * repositories, each with its own base branch. Empty selection = unbound
 * (repo-less task). Each bound repository renders one row (repo label +
 * base-branch select + remove); the add dropdown lists only repositories not
 * already bound, so a repository can never appear twice.
 */
export function WatcherRepositoryMultiFields({
  workspaceId,
  bindings,
  onChange,
}: {
  workspaceId: string;
  bindings: WatcherRepoBinding[];
  onChange: (bindings: WatcherRepoBinding[]) => void;
}) {
  const { t } = useTranslation();
  // forceRefresh: a repo created in settings doesn't update the shared store
  // slice, and the lazy fetch is gated by isLoaded — so without this the picker
  // could miss repositories created since the slice first loaded.
  const { repositories } = useRepositories(workspaceId, !!workspaceId, true);
  const labelById = new Map<string, string>(repositories.map((r) => [r.id, r.name]));
  const boundIds = new Set(bindings.map((b) => b.repositoryId));
  const available = repositories.filter((r) => !boundIds.has(r.id));

  const addBinding = (repositoryId: string) => {
    if (boundIds.has(repositoryId)) return;
    onChange([...bindings, { repositoryId, baseBranch: "" }]);
  };
  const changeBranch = (repositoryId: string, baseBranch: string) => {
    onChange(bindings.map((b) => (b.repositoryId === repositoryId ? { ...b, baseBranch } : b)));
  };
  const removeBinding = (repositoryId: string) => {
    onChange(bindings.filter((b) => b.repositoryId !== repositoryId));
  };

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>{t("linear:watcher.repositories")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("linear:watcher.repositoriesDescription")}
        </p>
      </div>
      {bindings.length === 0 && available.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("linear:watcher.noRepositories")}</p>
      ) : null}
      {bindings.map((binding) => (
        <RepoBindingRow
          key={binding.repositoryId}
          workspaceId={workspaceId}
          binding={binding}
          repositoryLabel={labelById.get(binding.repositoryId) ?? binding.repositoryId}
          onBranchChange={(baseBranch) => changeBranch(binding.repositoryId, baseBranch)}
          onRemove={() => removeBinding(binding.repositoryId)}
        />
      ))}
      {available.length > 0 ? (
        <AddRepositorySelect
          options={available.map((r) => ({ id: r.id, label: r.name }))}
          onAdd={addBinding}
          triggerLabel={t("linear:watcher.addRepository")}
          placeholder={t("linear:watcher.selectRepository")}
        />
      ) : null}
    </div>
  );
}

// RepoBindingRow renders one bound repository. It is its own component so the
// per-repo useBranches hook sits at a stable position (never inside a .map).
function RepoBindingRow({
  workspaceId,
  binding,
  repositoryLabel,
  onBranchChange,
  onRemove,
}: {
  workspaceId: string;
  binding: WatcherRepoBinding;
  repositoryLabel: string;
  onBranchChange: (baseBranch: string) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const branchSource = binding.repositoryId
    ? ({ kind: "id", workspaceId, repositoryId: binding.repositoryId } as const)
    : null;
  const { branches, isLoading } = useBranches(branchSource, !!binding.repositoryId);
  // A branch name can appear twice (local + remote tracking, e.g. "main" and
  // origin/"main"), which would emit two <SelectItem value="main"> — Radix then
  // renders every matching item's text in the trigger and React warns on
  // duplicate keys. Dedupe by name so each branch is one option.
  const uniqueBranchNames = Array.from(new Set(branches.map((b) => b.name)));
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,240px)] sm:items-end">
      <div className="min-w-0 space-y-1.5">
        <Label>{t("linear:watcher.repository")}</Label>
        <p className="truncate text-sm font-medium" title={repositoryLabel}>
          {repositoryLabel}
        </p>
      </div>
      <div className="flex items-end gap-2">
        <div className="min-w-0 flex-1 space-y-1.5">
          <Label>{t("linear:watcher.baseBranch")}</Label>
          <Select
            value={binding.baseBranch || DEFAULT_BRANCH}
            onValueChange={(v) => onBranchChange(resolveBaseBranch(v))}
            disabled={isLoading}
          >
            <SelectTrigger
              className="cursor-pointer"
              data-testid={`branch-trigger-${binding.repositoryId}`}
            >
              <SelectValue placeholder={t("linear:watcher.loadingBranches")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={DEFAULT_BRANCH}>{t("linear:watcher.defaultBranch")}</SelectItem>
              {uniqueBranchNames.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-9 w-9 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
          onClick={onRemove}
          aria-label={t("linear:watcher.removeRepository")}
          data-testid={`remove-repo-${binding.repositoryId}`}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function AddRepositorySelect({
  options,
  onAdd,
  triggerLabel,
  placeholder,
}: {
  options: { id: string; label: string }[];
  onAdd: (id: string) => void;
  triggerLabel: string;
  placeholder: string;
}) {
  // Controlled with a transient value: after each pick the select resets to
  // the placeholder so the same flow can repeat for the next repository.
  const [value, setValue] = useState("");
  return (
    <div className="space-y-1.5 sm:max-w-[280px]">
      <Label>{triggerLabel}</Label>
      <Select
        value={value || undefined}
        onValueChange={(v) => {
          setValue("");
          onAdd(v);
        }}
      >
        <SelectTrigger className="cursor-pointer" data-testid="add-repository-trigger">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.id} value={o.id}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
