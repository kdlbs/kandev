"use client";

import { useId, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { BranchSelector } from "@/components/branch-selector";
import { branchToOption, sortBranches } from "@/components/branch-picker-options";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  NO_REPOSITORY,
  DEFAULT_BRANCH,
  branchPlaceholder,
  resolveBaseBranch,
  resolveRepositoryId,
} from "@/lib/watcher-repository-default";
import { useTranslation } from "react-i18next";

type PickItem = { id: string; label: string };

function PickSelect(props: {
  label: string;
  description: string;
  value: string | undefined;
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

function storedBranchFallback(value: string) {
  const isOriginRef = value.startsWith("origin/");
  return branchToOption({
    name: isOriginRef ? value.slice("origin/".length) : value,
    type: isOriginRef ? "remote" : "local",
    remote: isOriginRef ? "origin" : undefined,
  });
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
  const { t } = useTranslation();
  // forceRefresh: a repo created in settings doesn't update the shared store
  // slice, and the lazy fetch is gated by isLoaded — so without this the picker
  // could miss repositories created since the slice first loaded. The hook owns
  // the fetch (keeps data-flow in the hook layer, not the component).
  const { repositories } = useRepositories(workspaceId, !!workspaceId, true);
  const branchSource = repositoryId ? ({ kind: "id", workspaceId, repositoryId } as const) : null;
  const {
    branches,
    isLoading: branchesLoading,
    refresh,
  } = useBranches(branchSource, !!repositoryId);
  const touchTarget = useTouchDrawer();
  const baseBranchId = useId();
  const noRepositoryLabel = t("common:noRepositoryOption");
  const defaultBranchLabel = t("common:repositoryDefaultBranchOption");
  const branchPlaceholderLabels = {
    defaultBranch: defaultBranchLabel,
    loading: t("common:watcherLoadingBranchesPlaceholder"),
    pickRepository: t("common:watcherPickRepositoryPlaceholder"),
  };
  const branchPlaceholderLabel =
    branchPlaceholderLabels[branchPlaceholder(repositoryId, branchesLoading)];
  const branchOptions = useMemo(() => {
    const seenValues = new Set<string>();
    const available = sortBranches(branches)
      .filter((branch) => branch.type !== "remote" || !branch.remote || branch.remote === "origin")
      .map(branchToOption)
      .filter((option) => {
        if (seenValues.has(option.value)) return false;
        seenValues.add(option.value);
        return true;
      });
    const projectedValues = new Set(available.map((option) => option.value));
    const storedFallback =
      baseBranch && !projectedValues.has(baseBranch) ? [storedBranchFallback(baseBranch)] : [];
    return [{ value: DEFAULT_BRANCH, label: defaultBranchLabel }, ...storedFallback, ...available];
  }, [baseBranch, branches, defaultBranchLabel]);
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <PickSelect
        label={t("common:repository")}
        description={t("common:optionalTheRepositoryTheAgentWorks")}
        value={repositoryId || NO_REPOSITORY}
        onChange={(v) => onRepositoryChange(resolveRepositoryId(v))}
        placeholder={noRepositoryLabel}
        items={[
          { id: NO_REPOSITORY, label: noRepositoryLabel },
          ...repositories.map((r) => ({ id: r.id, label: r.name })),
        ]}
      />
      <div className="space-y-1.5">
        <Label htmlFor={baseBranchId}>{t("common:baseBranch")}</Label>
        <p className="text-xs text-muted-foreground">{t("common:theBaseBranchTheAgentStarts")}</p>
        <BranchSelector
          options={branchOptions}
          value={repositoryId && !branchesLoading ? baseBranch || DEFAULT_BRANCH : ""}
          onValueChange={(value) => onBaseBranchChange(resolveBaseBranch(value))}
          disabled={!repositoryId || branchesLoading}
          placeholder={branchPlaceholderLabel}
          searchPlaceholder={t("task:searchBranches")}
          emptyMessage={t("task:noBranchesFound")}
          ariaLabel={t("common:baseBranch")}
          dropdownLabel={t("common:baseBranch")}
          onRefresh={refresh}
          refreshing={branchesLoading}
          loading={branchesLoading}
          touchTarget={touchTarget}
          triggerId={baseBranchId}
          testId="watcher-base-branch-selector"
          dropdownTestId="watcher-base-branch-dropdown"
          triggerClassName="border border-input bg-background px-3 hover:bg-background"
        />
      </div>
    </div>
  );
}
