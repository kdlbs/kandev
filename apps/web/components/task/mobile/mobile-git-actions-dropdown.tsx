"use client";

import {
  IconCloudDownload,
  IconCloudUpload,
  IconDots,
  IconEye,
  IconGitCherryPick,
  IconGitCommit,
  IconGitMerge,
  IconGitPullRequest,
  IconLoader2,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useChangeRequestTerminology } from "@/hooks/use-git-operations";
import { PushSubmenu } from "./mobile-git-push-submenu";

const DEFAULT_BASE_BRANCH = "main";

export type GitActionsDropdownProps = {
  sessionId: string | null | undefined;
  isGitLoading: boolean;
  uncommittedCount: number;
  baseBranch: string | undefined;
  onCommitClick: () => void;
  onPRClick: () => void;
  onPull: () => void;
  onPush: (force?: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
  pushDisabled?: boolean;
  pullDisabled?: boolean;
  showContributionResolution?: boolean;
  replaceDisabled?: boolean;
  useDisabled?: boolean;
  onReplaceContribution?: () => void;
  onUseContribution?: () => void;
  onViewPRVersion?: () => void;
};

function RebaseMergeItems({
  disabled,
  baseBranch,
  onRebase,
  onMerge,
}: {
  disabled: boolean;
  baseBranch: string | undefined;
  onRebase: () => void;
  onMerge: () => void;
}) {
  const { t } = useTranslation();
  // `main` is the default branch name, not copy — it travels through as an
  // interpolated value so it is never translated.
  const branch = baseBranch || DEFAULT_BASE_BRANCH;
  return (
    <>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onRebase}
        disabled={disabled}
      >
        <IconGitCherryPick className="h-4 w-4 text-orange-500" />
        <span className="flex-1">{t("task:rebase")}</span>
        <span className="text-xs text-muted-foreground">{t("task:ontoBranch", { branch })}</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onMerge}
        disabled={disabled}
      >
        <IconGitMerge className="h-4 w-4 text-purple-500" />
        <span className="flex-1">{t("task:merge")}</span>
        <span className="text-xs text-muted-foreground">{t("task:fromBranch", { branch })}</span>
      </DropdownMenuItem>
    </>
  );
}

function ContributionResolutionItems({
  disabled,
  replaceDisabled,
  useDisabled,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
}: Pick<
  GitActionsDropdownProps,
  | "replaceDisabled"
  | "useDisabled"
  | "onReplaceContribution"
  | "onUseContribution"
  | "onViewPRVersion"
> & { disabled: boolean }) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onReplaceContribution}
        disabled={disabled || replaceDisabled || !onReplaceContribution}
      >
        <IconCloudUpload className="h-4 w-4 text-orange-500" />
        <span className="flex-1">{t("task:replacePRBranch")}</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onUseContribution}
        disabled={disabled || useDisabled || !onUseContribution}
      >
        <IconGitCherryPick className="h-4 w-4 text-blue-500" />
        <span className="flex-1">{t("task:usePRVersion")}</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onViewPRVersion}
        disabled={disabled || !onViewPRVersion}
      >
        <IconEye className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("task:viewPRVersion")}</span>
      </DropdownMenuItem>
    </>
  );
}

function StandardGitActionItems({
  disabled,
  pullDisabled,
  pushDisabled,
  onPull,
  onPush,
}: Pick<GitActionsDropdownProps, "pullDisabled" | "pushDisabled" | "onPull" | "onPush"> & {
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuItem
        className="min-h-11 cursor-pointer gap-3"
        onClick={onPull}
        disabled={disabled || pullDisabled}
        title={pullDisabled ? t("task:providerHistoryUnavailable") : undefined}
      >
        <IconCloudDownload className="h-4 w-4 text-blue-500" />
        <span className="flex-1">{t("task:pull")}</span>
      </DropdownMenuItem>
      <PushSubmenu
        disabled={disabled || (pushDisabled ?? false)}
        disabledReason={pushDisabled ? t("task:providerHistoryUnavailable") : undefined}
        onPush={onPush}
      />
    </>
  );
}

export function GitActionsDropdown({
  sessionId,
  isGitLoading,
  uncommittedCount,
  baseBranch,
  onCommitClick,
  onPRClick,
  onPull,
  onPush,
  onRebase,
  onMerge,
  pushDisabled = false,
  pullDisabled = false,
  showContributionResolution = false,
  replaceDisabled = false,
  useDisabled = false,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
}: GitActionsDropdownProps) {
  const { t } = useTranslation();
  const disabled = isGitLoading || !sessionId;
  const terminology = useChangeRequestTerminology(sessionId);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="icon-sm"
          variant="ghost"
          className="h-11 w-11 cursor-pointer"
          aria-label={t("task:gitActions")}
          data-testid="mobile-git-actions"
        >
          {isGitLoading ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconDots className="h-4 w-4" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuItem
          className="min-h-11 cursor-pointer gap-3"
          onClick={onCommitClick}
          disabled={disabled}
        >
          <IconGitCommit className={`h-4 w-4 ${uncommittedCount > 0 ? "text-amber-500" : ""}`} />
          <span className="flex-1">{t("task:commit")}</span>
          {uncommittedCount > 0 && (
            <span className="rounded-full bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium text-amber-600">
              {uncommittedCount}
            </span>
          )}
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-11 cursor-pointer gap-3"
          onClick={onPRClick}
          disabled={disabled}
        >
          <IconGitPullRequest className="h-4 w-4 text-cyan-500" />
          <span className="flex-1">
            {t("task:createChangeRequest", { shortName: terminology.shortName })}
          </span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {showContributionResolution ? (
          <ContributionResolutionItems
            disabled={disabled}
            replaceDisabled={replaceDisabled}
            useDisabled={useDisabled}
            onReplaceContribution={onReplaceContribution}
            onUseContribution={onUseContribution}
            onViewPRVersion={onViewPRVersion}
          />
        ) : (
          <StandardGitActionItems
            disabled={disabled}
            pullDisabled={pullDisabled}
            pushDisabled={pushDisabled}
            onPull={onPull}
            onPush={onPush}
          />
        )}
        <DropdownMenuSeparator />
        <RebaseMergeItems
          disabled={disabled}
          baseBranch={baseBranch}
          onRebase={onRebase}
          onMerge={onMerge}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
