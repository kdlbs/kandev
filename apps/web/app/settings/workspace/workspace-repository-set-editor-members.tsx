"use client";

import { useMemo, useState } from "react";
import { IconArrowDown, IconArrowUp, IconRefresh, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { BranchSelector } from "@/components/task-create-dialog-selectors";
import { branchToOption, sortBranches } from "@/components/task-create-dialog-branch-options";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import type { Repository } from "@/lib/types/http";
import type { RepositorySetDraft, RepositorySetDraftMember } from "./use-workspace-repository-sets";

type RepositorySetMembersFieldProps = {
  workspaceId: string;
  draft: RepositorySetDraft;
  repositories: Repository[];
  onChange: (patch: Partial<RepositorySetDraft>) => void;
};

function useRepositorySetMemberActions({
  draft,
  repositories,
  onChange,
}: Omit<RepositorySetMembersFieldProps, "workspaceId">) {
  const [memberSearch, setMemberSearch] = useState("");
  const selectedIds = useMemo(
    () => new Set(draft.members.map((member) => member.repositoryId)),
    [draft.members],
  );
  const availableRepositories = useMemo(
    () => repositories.filter((repository) => !selectedIds.has(repository.id as string)),
    [repositories, selectedIds],
  );
  const visibleMembers = useMemo(() => {
    const query = memberSearch.trim().toLowerCase();
    return draft.members
      .map((member, index) => ({ member, index }))
      .filter(({ member }) => {
        if (!query) return true;
        const repository = repositories.find((candidate) => candidate.id === member.repositoryId);
        return [repository?.name, repository?.local_path, member.baseBranch]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(query));
      });
  }, [draft.members, memberSearch, repositories]);

  const addRepository = (repositoryId: string) => {
    if (!repositoryId || selectedIds.has(repositoryId)) return;
    onChange({ members: [...draft.members, { repositoryId, baseBranch: "" }] });
  };
  const updateMember = (index: number, patch: Partial<RepositorySetDraftMember>) => {
    onChange({
      members: draft.members.map((member, memberIndex) =>
        memberIndex === index ? { ...member, ...patch } : member,
      ),
    });
  };
  const removeMember = (index: number) => {
    onChange({ members: draft.members.filter((_, memberIndex) => memberIndex !== index) });
  };
  const moveMember = (index: number, delta: number) => {
    const target = index + delta;
    if (target < 0 || target >= draft.members.length) return;
    const members = [...draft.members];
    [members[index], members[target]] = [members[target], members[index]];
    onChange({ members });
  };
  const resetBases = () => {
    onChange({ members: draft.members.map((member) => ({ ...member, baseBranch: "" })) });
  };

  return {
    memberSearch,
    setMemberSearch,
    availableRepositories,
    visibleMembers,
    addRepository,
    updateMember,
    removeMember,
    moveMember,
    resetBases,
    hasSavedBases: draft.members.some((member) => member.baseBranch !== ""),
  };
}

export function RepositorySetMembersField({
  workspaceId,
  draft,
  repositories,
  onChange,
}: RepositorySetMembersFieldProps) {
  const { t } = useTranslation();
  const {
    memberSearch,
    setMemberSearch,
    availableRepositories,
    visibleMembers,
    addRepository,
    updateMember,
    removeMember,
    moveMember,
    resetBases,
    hasSavedBases,
  } = useRepositorySetMemberActions({ draft, repositories, onChange });

  return (
    <div className="space-y-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-xs font-medium">{t("workspaces:repositorySetsMembersLabel")}</p>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:repositorySetsMembersHint")}
          </p>
        </div>
        <Combobox
          options={availableRepositories.map(repositoryOption)}
          value=""
          onValueChange={addRepository}
          placeholder={t("workspaces:repositorySetsAddRepository")}
          searchPlaceholder={t("workspaces:repositorySetsSearchRepositories")}
          emptyMessage={t("workspaces:repositorySetsNoMatchingRepositories")}
          disabled={availableRepositories.length === 0}
          triggerClassName="min-h-11 border border-input bg-background px-3"
          testId="repository-set-add-repository"
          dropdownTestId="repository-set-add-repository-dropdown"
          ariaLabel={t("workspaces:repositorySetsAddRepository")}
        />
      </div>
      {draft.members.length > 0 ? (
        <div className="space-y-2">
          <Input
            value={memberSearch}
            onChange={(event) => setMemberSearch(event.target.value)}
            placeholder={t("workspaces:repositorySetsSearchSelected")}
            aria-label={t("workspaces:repositorySetsSearchSelected")}
            data-testid="repository-set-member-search"
          />
          <div className="space-y-2" data-testid="repository-set-members">
            {visibleMembers.length === 0 ? (
              <p className="rounded-md border px-3 py-3 text-xs text-muted-foreground">
                {t("workspaces:repositorySetsNoMatchingMembers")}
              </p>
            ) : (
              visibleMembers.map(({ member, index }) => (
                <RepositorySetMemberRow
                  key={member.repositoryId}
                  workspaceId={workspaceId}
                  member={member}
                  repository={repositories.find(
                    (candidate) => candidate.id === member.repositoryId,
                  )}
                  canMoveUp={!memberSearch && index > 0}
                  canMoveDown={!memberSearch && index < draft.members.length - 1}
                  onBaseBranchChange={(baseBranch) => updateMember(index, { baseBranch })}
                  onMoveUp={() => moveMember(index, -1)}
                  onMoveDown={() => moveMember(index, 1)}
                  onRemove={() => removeMember(index)}
                />
              ))
            )}
          </div>
        </div>
      ) : (
        <p className="rounded-md border border-dashed px-3 py-4 text-xs text-muted-foreground">
          {t("workspaces:repositorySetsNoMembersSelected")}
        </p>
      )}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="min-h-11 cursor-pointer"
        disabled={!hasSavedBases}
        onClick={resetBases}
        data-testid="repository-set-reset-bases"
      >
        <IconRefresh className="mr-1.5 h-4 w-4" />
        {t("workspaces:repositorySetsResetBases")}
      </Button>
    </div>
  );
}

function repositoryOption(repository: Repository): ComboboxOption {
  const label = repository.name || repository.provider_name || (repository.id as string);
  const description = repository.local_path || repository.remote_url || "";
  return {
    value: repository.id as string,
    label,
    description,
    keywords: [label, description, repository.provider_owner, repository.provider_name].filter(
      Boolean,
    ),
    renderLabel: () => (
      <span className="flex min-w-0 flex-col">
        <span className="truncate">{label}</span>
        {description ? (
          <span className="truncate text-xs text-muted-foreground">{description}</span>
        ) : null}
      </span>
    ),
  };
}

type RepositorySetMemberRowProps = {
  workspaceId: string;
  member: RepositorySetDraftMember;
  repository?: Repository;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onBaseBranchChange: (value: string) => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onRemove: () => void;
};

function RepositorySetMemberRow({
  workspaceId,
  member,
  repository,
  canMoveUp,
  canMoveDown,
  onBaseBranchChange,
  onMoveUp,
  onMoveDown,
  onRemove,
}: RepositorySetMemberRowProps) {
  const { t } = useTranslation();
  return (
    <div
      className="grid gap-2 rounded-md border p-2 sm:grid-cols-[minmax(0,1fr)_minmax(14rem,auto)_auto] sm:items-center"
      data-testid={`repository-set-member-${member.repositoryId}`}
    >
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{repository?.name ?? member.repositoryId}</p>
        {repository?.local_path ? (
          <p className="truncate text-xs text-muted-foreground" title={repository.local_path}>
            {repository.local_path}
          </p>
        ) : null}
      </div>
      {repository ? (
        <RepositorySetBaseBranchPicker
          workspaceId={workspaceId}
          member={member}
          repository={repository}
          onChange={onBaseBranchChange}
        />
      ) : (
        <p className="text-xs text-destructive">{t("workspaces:repositorySetsMemberMissing")}</p>
      )}
      <div className="flex justify-end gap-0.5">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-11 min-w-11 cursor-pointer"
          disabled={!canMoveUp}
          aria-label={t("workspaces:repositorySetsMoveUp")}
          onClick={onMoveUp}
          data-testid={`repository-set-move-up-${member.repositoryId}`}
        >
          <IconArrowUp className="h-4 w-4" />
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-11 min-w-11 cursor-pointer"
          disabled={!canMoveDown}
          aria-label={t("workspaces:repositorySetsMoveDown")}
          onClick={onMoveDown}
          data-testid={`repository-set-move-down-${member.repositoryId}`}
        >
          <IconArrowDown className="h-4 w-4" />
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-11 min-w-11 cursor-pointer"
          aria-label={t("workspaces:repositorySetsRemove")}
          onClick={onRemove}
          data-testid={`repository-set-remove-${member.repositoryId}`}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function taskDefaultOption(repository: Repository, t: TFunction) {
  const label = t("workspaces:repositorySetsTaskDefault", {
    branch: repository.default_branch,
  });
  return {
    value: "",
    label,
    keywords: [label, repository.default_branch].filter(Boolean),
    renderLabel: () => <span className="truncate">{label}</span>,
    renderTriggerLabel: () => <span className="truncate">{label}</span>,
  } satisfies ComboboxOption;
}

function unavailableBranchOption(
  branch: string,
  unavailableLabel: string,
  unavailableReason: string,
): ComboboxOption {
  return {
    value: branch,
    label: branch,
    keywords: [branch, unavailableLabel],
    disabled: true,
    disabledReason: unavailableReason,
    renderLabel: () => (
      <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
        <span className="truncate">{branch}</span>
        <Badge variant="destructive" className="shrink-0 text-xs">
          {unavailableLabel}
        </Badge>
      </span>
    ),
  };
}

function RepositorySetBaseBranchPicker({
  workspaceId,
  member,
  repository,
  onChange,
}: {
  workspaceId: string;
  member: RepositorySetDraftMember;
  repository: Repository;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [branchOpen, setBranchOpen] = useState(false);
  const { branches, isLoaded, isLoading, refresh } = useBranches(
    { kind: "id", workspaceId, repositoryId: repository.id as string },
    branchOpen,
  );
  const options = useMemo(() => {
    const available = sortBranches(branches).map(branchToOption);
    const defaultOption = taskDefaultOption(repository, t);
    const savedBaseAvailable = available.some((option) => option.value === member.baseBranch);
    if (member.baseBranch && !savedBaseAvailable) {
      const savedBaseOption = isLoaded
        ? unavailableBranchOption(
            member.baseBranch,
            t("workspaces:repositorySetsBranchUnavailable"),
            t("workspaces:repositorySetsBranchUnavailableReason"),
          )
        : {
            value: member.baseBranch,
            label: member.baseBranch,
            keywords: [member.baseBranch],
          };
      return [savedBaseOption, defaultOption, ...available];
    }
    return [defaultOption, ...available];
  }, [branches, isLoaded, member.baseBranch, repository, t]);

  return (
    <BranchSelector
      options={options}
      value={member.baseBranch}
      onValueChange={onChange}
      disabled={false}
      placeholder={t("workspaces:repositorySetsTaskDefault", {
        branch: repository.default_branch,
      })}
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={t("task:noBranches")}
      onRefresh={refresh ? () => void refresh() : undefined}
      refreshing={isLoading}
      loading={isLoading}
      ariaLabel={t("workspaces:repositorySetsBaseBranchFor", { name: repository.name })}
      dropdownLabel={t("workspaces:repositorySetsBaseBranchLabel")}
      testId={`repository-set-base-${member.repositoryId}`}
      dropdownTestId={`repository-set-base-dropdown-${member.repositoryId}`}
      triggerClassName="min-h-11 border border-input bg-background px-3 hover:bg-background"
      onOpenChange={setBranchOpen}
    />
  );
}
