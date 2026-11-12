"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  useRemoteRepositories,
  type RemoteRepository,
} from "@/hooks/domains/integrations/use-remote-repositories";
import {
  RemoteRepositoryPickerPanel,
  type RemoteRepositoryTextSubmitResult,
} from "@/components/remote-repository-picker";
import { parseGitHubRepositoryInput } from "@/lib/github/repository-input";
import { repoAllowedByScope, sameRepoFilter } from "@/lib/github/repo-scope";
import type { GitHubWorkspaceSettings, RepoFilter } from "@/lib/types/github";
import { SelectionAddButton, SelectionChip } from "./selection-chip";

// Stable module-level reference so the hook's provider restriction is not rebuilt per render.
const GITHUB_ONLY = { providers: ["github"] } as const;

export type GitHubRepositoryMultiSelectProps = {
  workspaceId: string | undefined;
  selected: RepoFilter[];
  onChange: (repos: RepoFilter[]) => void;
  /** When set, only repositories this workspace scope allows can be added. */
  scope?: GitHubWorkspaceSettings | null;
  disabled?: boolean;
};

function filterLabel(filter: RepoFilter): string {
  return filter.name === "" ? `${filter.owner}/*` : `${filter.owner}/${filter.name}`;
}

/**
 * Selected GitHub repositories as removable chips, plus an "Add repository"
 * control that opens the shared remote repository picker limited to GitHub.
 */
export function GitHubRepositoryMultiSelect({
  workspaceId,
  selected,
  onChange,
  scope,
  disabled = false,
}: GitHubRepositoryMultiSelectProps) {
  const remove = useCallback(
    (filter: RepoFilter) => onChange(selected.filter((entry) => !sameRepoFilter(entry, filter))),
    [onChange, selected],
  );
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      {selected.map((filter) => (
        <SelectedRepositoryChip
          key={filterLabel(filter)}
          filter={filter}
          disabled={disabled}
          onRemove={remove}
        />
      ))}
      <AddGitHubRepository
        workspaceId={workspaceId}
        selected={selected}
        onChange={onChange}
        scope={scope}
        disabled={disabled}
      />
    </div>
  );
}

function SelectedRepositoryChip({
  filter,
  disabled,
  onRemove,
}: {
  filter: RepoFilter;
  disabled: boolean;
  onRemove: (filter: RepoFilter) => void;
}) {
  const { t } = useTranslation();
  const label = filterLabel(filter);
  return (
    <SelectionChip
      label={label}
      removeLabel={t("github:removeRepositoryFilter", { repository: label })}
      testId="github-repository-chip"
      disabled={disabled}
      onRemove={() => onRemove(filter)}
    />
  );
}

function AddGitHubRepository({
  workspaceId,
  selected,
  onChange,
  scope,
  disabled,
}: Required<Omit<GitHubRepositoryMultiSelectProps, "scope">> & {
  scope?: GitHubWorkspaceSettings | null;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const accessible = useRemoteRepositories(workspaceId ?? "", GITHUB_ONLY);
  const isSelected = (filter: RepoFilter) =>
    selected.some((entry) => sameRepoFilter(entry, filter));
  const add = (filter: RepoFilter) => {
    if (!isSelected(filter)) onChange([...selected, filter]);
    setOpen(false);
  };
  const submitText = (value: string): RemoteRepositoryTextSubmitResult => {
    const parsed = parseGitHubRepositoryInput(value);
    if (!parsed) return { status: "ignored" };
    if (!repoAllowedByScope(scope, parsed.owner, parsed.name)) {
      return { status: "rejected", message: t("github:repositoryOutsideWorkspaceScope") };
    }
    add(parsed);
    return { status: "accepted" };
  };
  const toFilter = (repo: RemoteRepository): RepoFilter => ({ owner: repo.owner, name: repo.name });
  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <SelectionAddButton
          label={t("github:addRepository")}
          testId="github-repository-add"
          disabled={disabled}
        />
      </PopoverTrigger>
      <PopoverContent
        className="w-[380px] max-w-[calc(100vw-2rem)] max-h-[min(420px,calc(100vh-12rem))] overflow-hidden p-0"
        align="start"
        collisionPadding={8}
        portal={false}
        data-testid="github-repository-picker"
      >
        <RemoteRepositoryPickerPanel
          accessible={accessible}
          onPick={(repo) => add(toFilter(repo))}
          isRepoSelected={(repo) => isSelected(toFilter(repo))}
          isRepoAllowed={(repo) => repoAllowedByScope(scope, repo.owner, repo.name)}
          isTextSubmittable={(value) => parseGitHubRepositoryInput(value) !== null}
          onSubmitText={submitText}
          stagedHint={<StagedRepositoryHint />}
          placeholder={t("github:searchRepositoriesOrEnterOwnerRepo", { format: "owner/repo" })}
        />
      </PopoverContent>
    </Popover>
  );
}

function StagedRepositoryHint() {
  const { t } = useTranslation();
  return (
    <div className="px-2 pt-1 text-xs text-muted-foreground">
      {t("github:pressEnterToAddRepository")}
    </div>
  );
}
