"use client";

import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCloudDownload,
  IconCloudUpload,
  IconGitCherryPick,
  IconGitMerge,
  IconGitPullRequest,
} from "@tabler/icons-react";
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import { AHEAD_MARK, BEHIND_MARK, DEFAULT_BASE_BRANCH } from "./vcs-constants";
import { RemoteContributionActionItems } from "@/components/task/remote-contribution-action-items";

function StandardPushDropdownItems({
  disabled,
  hasUpstream,
  aheadCount,
  pushDisabled,
  onPush,
  disabledTitle,
}: {
  disabled: boolean;
  hasUpstream: boolean;
  aheadCount: number;
  pushDisabled: boolean;
  onPush: (force: boolean) => void;
  disabledTitle: string;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        className="cursor-pointer gap-3"
        disabled={disabled || pushDisabled}
        title={pushDisabled ? disabledTitle : undefined}
      >
        <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:push")}</span>
        {hasUpstream && aheadCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            {AHEAD_MARK}
            {aheadCount}
          </span>
        )}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(false)}
          disabled={disabled || pushDisabled}
          title={pushDisabled ? disabledTitle : undefined}
        >
          <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
          <span>{t("integrations:push")}</span>
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(true)}
          disabled={disabled || pushDisabled}
          title={pushDisabled ? disabledTitle : undefined}
        >
          <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
          <span>{t("integrations:forcePush")}</span>
        </DropdownMenuItem>
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

function ContributionDropdownItems({
  disabled,
  replaceDisabled,
  useDisabled,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
  prNumber,
}: {
  disabled: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  onReplaceContribution: () => void;
  onUseContribution: () => void;
  onViewPRVersion: () => void;
  prNumber?: number;
}) {
  return (
    <>
      <DropdownMenuSeparator />
      <RemoteContributionActionItems
        disabled={disabled}
        replaceDisabled={replaceDisabled}
        useDisabled={useDisabled}
        onReplaceContribution={onReplaceContribution}
        onUseContribution={onUseContribution}
        onViewPRVersion={onViewPRVersion}
        prNumber={prNumber}
      />
    </>
  );
}

export type VcsDropdownItemsProps = {
  disabled: boolean;
  baseBranch?: string;
  hasUpstream: boolean;
  behindCount: number;
  aheadCount: number;
  pushDisabled: boolean;
  pullDisabled: boolean;
  pushDisabledReason?: string;
  pullDisabledReason?: string;
  showContributionResolution: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onReplaceContribution: () => void;
  onUseContribution: () => void;
  onViewPRVersion: () => void;
  prNumber?: number;
  onRebase: () => void;
  onMerge: () => void;
};

export function VcsDropdownItems({
  disabled,
  baseBranch,
  hasUpstream,
  behindCount,
  aheadCount,
  pushDisabled,
  pullDisabled,
  pushDisabledReason,
  pullDisabledReason,
  showContributionResolution,
  replaceDisabled,
  useDisabled,
  onPR,
  onPull,
  onPush,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
  prNumber,
  onRebase,
  onMerge,
}: VcsDropdownItemsProps) {
  const { t } = useTranslation();
  const pushDisabledTitle = pushDisabledReason ?? t("task:divergedActionsUnavailable");
  const pullDisabledTitle = pullDisabledReason ?? t("task:divergedActionsUnavailable");
  return (
    <DropdownMenuContent align="end" className="w-56">
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPR} disabled={disabled}>
        <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:createPr")}</span>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        className="cursor-pointer gap-3"
        onClick={onPull}
        disabled={disabled || pullDisabled}
        title={pullDisabled ? pullDisabledTitle : undefined}
      >
        <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:pull")}</span>
        {hasUpstream && behindCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            {BEHIND_MARK}
            {behindCount}
          </span>
        )}
      </DropdownMenuItem>
      {!showContributionResolution && (
        <StandardPushDropdownItems
          disabled={disabled}
          hasUpstream={hasUpstream}
          aheadCount={aheadCount}
          pushDisabled={pushDisabled}
          onPush={onPush}
          disabledTitle={pushDisabledTitle}
        />
      )}
      {showContributionResolution && (
        <ContributionDropdownItems
          disabled={disabled}
          replaceDisabled={replaceDisabled}
          useDisabled={useDisabled}
          onReplaceContribution={onReplaceContribution}
          onUseContribution={onUseContribution}
          onViewPRVersion={onViewPRVersion}
          prNumber={prNumber}
        />
      )}
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onRebase} disabled={disabled}>
        <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:rebase")}</span>
        <span className="text-xs text-muted-foreground">
          {t("integrations:ontoBranch", { branch: baseBranch || DEFAULT_BASE_BRANCH })}
        </span>
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onMerge} disabled={disabled}>
        <IconGitMerge className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:merge")}</span>
        <span className="text-xs text-muted-foreground">
          {t("integrations:fromBranch", { branch: baseBranch || DEFAULT_BASE_BRANCH })}
        </span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  );
}
