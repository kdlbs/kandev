"use client";

import { useMemo, useState } from "react";
import { IconCode, IconFolderPlus, IconGitBranch, IconInfoCircle } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Pill, type PillAction, type PillOption } from "@/components/task-create-dialog-pill";
import type { Branch, RepositoryBranchPolicy } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import {
  branchOptionValue,
  computeBranchPlaceholder,
} from "@/components/task-create-dialog-branch-options";
import {
  computeBranchDisabledReason,
  computeBranchPrefix,
  computeBranchTooltip,
  type BranchIntent,
} from "@/components/task-create-dialog-branch-utils";
import { scoreBranch } from "@/lib/utils/branch-filter";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

function BranchPolicyOptionInfo({
  policy,
  summary,
  unavailableReason,
}: {
  policy: RepositoryBranchPolicy;
  summary: string;
  unavailableReason?: string;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const details = unavailableReason ? `${summary} ${unavailableReason}` : summary;
  const trigger = (
    <button
      type="button"
      aria-label={details}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
      data-testid={`branch-policy-option-info-${policy.id}`}
      className="flex h-11 w-11 shrink-0 cursor-help items-center justify-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:h-7 sm:w-7"
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
    >
      <IconInfoCircle className="h-4 w-4" aria-hidden="true" />
    </button>
  );

  if (!usesTouchDrawer) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{trigger}</TooltipTrigger>
        <TooltipContent className="max-w-80 text-xs">{details}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{policy.name}</DrawerTitle>
          <DrawerDescription>{details}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

function branchPolicyToOption(
  policy: RepositoryBranchPolicy,
  branches: Branch[],
  policyDisabledReason?: string,
): PillOption {
  const baseBranchAvailable = branches.some(
    (branch) => branchOptionValue(branch) === policy.base_branch,
  );
  const unavailableReason =
    policyDisabledReason ?? (baseBranchAvailable ? undefined : t("task:branchPolicyUnavailable"));
  const summary = t("workspaces:branchPolicySummary", {
    base: policy.base_branch,
    template: policy.branch_template,
    target: policy.pull_request_target,
  });
  return {
    value: `policy:${policy.id}`,
    label: policy.name,
    keywords: [
      policy.name,
      policy.description,
      policy.base_branch,
      policy.branch_template,
      policy.pull_request_target,
    ].filter(Boolean),
    group: "policies",
    groupLabel: t("task:branchPoliciesGroup"),
    disabled: Boolean(unavailableReason),
    disabledReason: unavailableReason,
    renderLabel: () => (
      <span className="flex min-w-0 flex-1 items-center gap-2">
        <Badge variant="secondary" className="shrink-0 text-xs">
          {t("task:branchPolicyMarker")}
        </Badge>
        <span className="truncate" title={policy.name}>
          {policy.name}
        </span>
      </span>
    ),
    renderAccessory: () => (
      <BranchPolicyOptionInfo
        policy={policy}
        summary={summary}
        unavailableReason={unavailableReason}
      />
    ),
  };
}

export function useRepoChipBranchPicker({
  row,
  branchValue = row.branch,
  branchPolicies,
  branches,
  branchOptions,
  branchesLoading,
  preferredDefaultBranchLoading,
  policyDisabledReason,
  onBranchChange,
  onPolicyChange,
  onPolicySelected,
}: {
  row: TaskRepoRow;
  branchValue?: string;
  branchPolicies: RepositoryBranchPolicy[];
  branches: Branch[];
  branchOptions: PillOption[];
  branchesLoading: boolean;
  preferredDefaultBranchLoading?: boolean;
  policyDisabledReason?: string;
  onBranchChange: (value: string) => void;
  onPolicyChange?: (policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
}) {
  const { t } = useTranslation();
  const hasRepo = !!(row.repositoryId || row.localPath);
  const visibleBranchValue = preferredDefaultBranchLoading ? "" : branchValue;
  const selectedPolicy = branchPolicies.find((policy) => policy.id === row.branchPolicyId);
  const policyOptions = useMemo(
    () =>
      branchPolicies.map((policy) => branchPolicyToOption(policy, branches, policyDisabledReason)),
    [branchPolicies, branches, policyDisabledReason],
  );
  const branchPickerOptions = useMemo(
    () => [...policyOptions, ...branchOptions],
    [branchOptions, policyOptions],
  );
  const selectedBranchValue = selectedPolicy ? "policy:" + selectedPolicy.id : visibleBranchValue;
  const selectedBranchLabel = selectedPolicy
    ? t("task:branchPolicySelected", {
        name: selectedPolicy.name,
        base: selectedPolicy.base_branch,
      })
    : visibleBranchValue;
  const handleBranchSelect = (value: string) => {
    if (value.startsWith("policy:")) {
      const policy = branchPolicies.find((candidate) => `policy:${candidate.id}` === value);
      if (policy) {
        onPolicyChange?.(policy.id, policy.base_branch);
        onPolicySelected?.();
      }
      return;
    }
    onBranchChange(value);
  };
  const branchPlaceholder = computeBranchPlaceholder(
    hasRepo,
    branchesLoading || !!preferredDefaultBranchLoading,
    branchPickerOptions.length,
  );
  return {
    hasRepo,
    branchPickerOptions,
    selectedBranchValue,
    selectedBranchLabel,
    handleBranchSelect,
    branchPlaceholder,
  };
}

export type RepoChipBranchPicker = ReturnType<typeof useRepoChipBranchPicker>;

function buildCreateRepositoryAction(onSelect?: () => void): PillAction | undefined {
  if (!onSelect) return undefined;
  return {
    label: t("task:createNewRepository"),
    icon: <IconFolderPlus className="h-3.5 w-3.5" />,
    onSelect,
  };
}

export function RepoChipRepositoryPill({
  repoLabel,
  repoTooltip,
  repositoryValue,
  repoOptions,
  onRepositoryChange,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
}: {
  repoLabel: string;
  repoTooltip: string;
  repositoryValue: string;
  repoOptions: PillOption[];
  onRepositoryChange: (value: string) => void;
  onCreateRepository?: () => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Pill
      icon={<IconCode className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={repoLabel}
      selectedValue={repositoryValue}
      placeholder={t("task:repository")}
      options={repoOptions}
      onSelect={onRepositoryChange}
      searchPlaceholder={t("task:searchRepositories")}
      emptyMessage={t("task:noRepositories")}
      testId="repo-chip-trigger"
      tooltip={repoTooltip}
      action={buildCreateRepositoryAction(onCreateRepository)}
      onRefresh={onRefreshRepositories}
      refreshing={repositoriesRefreshing}
      refreshLabel="repositories"
      flat
    />
  );
}

export function RepoChipBranchPill({
  branchPicker,
  branchIntent,
  branchLocked,
  branchesLoading,
  refreshBranches,
}: {
  branchPicker: RepoChipBranchPicker;
  branchIntent?: BranchIntent;
  branchLocked?: boolean;
  branchesLoading: boolean;
  refreshBranches?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Pill
      icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={branchPicker.selectedBranchLabel}
      selectedValue={branchPicker.selectedBranchValue}
      placeholder={branchPicker.branchPlaceholder}
      prefix={computeBranchPrefix(branchIntent ?? "none")}
      options={branchPicker.branchPickerOptions}
      onSelect={branchPicker.handleBranchSelect}
      disabled={
        branchLocked ||
        !branchPicker.hasRepo ||
        branchesLoading ||
        branchPicker.branchPickerOptions.length === 0
      }
      disabledReason={computeBranchDisabledReason({
        branchLocked: !!branchLocked,
        hasRepo: branchPicker.hasRepo,
        branchesLoading,
        optionCount: branchPicker.branchPickerOptions.length,
      })}
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={t("task:noBranches")}
      testId="branch-chip-trigger"
      tooltip={computeBranchTooltip(branchIntent)}
      onRefresh={refreshBranches}
      refreshing={branchesLoading}
      filter={scoreBranch}
      flat
    />
  );
}

export function RepoChipBaseBranchPill({
  options,
  value,
  defaultBranch,
  hasRepo,
  branchesLoading,
  onSelect,
  refreshBranches,
}: {
  options: PillOption[];
  value: string;
  defaultBranch: string;
  hasRepo: boolean;
  branchesLoading: boolean;
  onSelect: (value: string) => void;
  refreshBranches?: () => void;
}) {
  const { t } = useTranslation();
  const defaultLabel = defaultBranch || t("common:repositoryDefaultBranchOption");
  const valueLabel = value || t("workspaces:repositorySetsTaskDefault", { branch: defaultLabel });
  const disabledReason = baseBranchDisabledReason(hasRepo, branchesLoading, t);
  return (
    <Pill
      icon={<IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />}
      value={valueLabel}
      selectedValue={value}
      placeholder={valueLabel}
      options={options}
      onSelect={onSelect}
      disabled={!hasRepo || branchesLoading || options.length === 0}
      disabledReason={disabledReason}
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={t("task:noBranches")}
      testId="repo-chip-base-branch"
      tooltip={t("workspaces:repositorySetsBaseBranchLabel")}
      onRefresh={refreshBranches}
      refreshing={branchesLoading}
      filter={scoreBranch}
      flat
    />
  );
}

function baseBranchDisabledReason(
  hasRepo: boolean,
  branchesLoading: boolean,
  translate: (key: string) => string,
): string {
  if (!hasRepo) return translate("task:selectRepositoryFirst");
  if (branchesLoading) return translate("task:loadingBranches2");
  return translate("task:noBranches");
}
