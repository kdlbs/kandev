"use client";

import {
  IconAlertTriangle,
  IconCloudDownload,
  IconCloudUpload,
  IconChevronDown,
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
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ReactNode } from "react";

export type PerRepoStatus = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  pushAhead: number;
  pullBehind: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

export type PerRepoCallbacks = {
  onCommit: (repo: string) => void;
  onPR: (repo: string) => void;
  onPull: (repo: string) => void;
  onPush: (force: boolean, repo: string) => void;
  onRebase: (repo: string) => void;
  onMerge: (repo: string) => void;
};

export type PrimaryButtonConfig = {
  icon: ReactNode;
  label: string;
  badge: number | null;
  tooltip: string;
};

/**
 * One complete sentence per action rather than a single frame with the action
 * name interpolated. `primaryButtonConfig.label` is itself already translated,
 * and a translated value dropped into a translated frame cannot be declined to
 * agree with the surrounding grammar (genitive in German/Russian, and so on) —
 * nor can the frame be reordered around it. Keyed off `primaryAction`, which is
 * an untranslated discriminant.
 */
const PICK_REPOSITORY_KEYS: Record<"commit" | "push" | "pr" | "rebase", string> = {
  commit: "integrations:pickARepositoryForCommit",
  push: "integrations:pickARepositoryForPush",
  pr: "integrations:pickARepositoryForCreatePr",
  rebase: "integrations:pickARepositoryForRebase",
};

const ITEM_CLASS = "cursor-pointer gap-3";
const ICON_CLASS = "h-4 w-4 text-muted-foreground";

export type VcsActionKey = "commit" | "push" | "pr" | "pull" | "rebase" | "merge" | "force-push";

export function isRemoteActionBlockedForRepository(
  action: VcsActionKey,
  repositoryName: string,
  blockedRepositoryName: string | undefined,
  pushDisabled: boolean,
  pullDisabled: boolean,
): boolean {
  const actionBlocked =
    (action === "push" || action === "force-push" ? pushDisabled : false) ||
    (action === "pull" ? pullDisabled : false);
  return actionBlocked && (!blockedRepositoryName || repositoryName === blockedRepositoryName);
}

type ActionDef = {
  key: VcsActionKey;
  /** Catalog key, resolved at render: `t()` here would freeze at the boot locale. */
  labelKey: string;
  icon: ReactNode;
  /** Returns true when this action should be disabled for a given repo. */
  disabledFor: (status: PerRepoStatus | undefined) => boolean;
  invoke: (repo: string, callbacks: PerRepoCallbacks) => void;
};

const ACTION_DEFS: ActionDef[] = [
  {
    key: "commit",
    labelKey: "integrations:commit",
    icon: <IconGitCommit className={ICON_CLASS} />,
    disabledFor: (s) => !((s?.hasStaged ?? false) || (s?.hasUnstaged ?? false)),
    invoke: (repo, cb) => cb.onCommit(repo),
  },
  {
    key: "push",
    labelKey: "integrations:push",
    icon: <IconCloudUpload className={ICON_CLASS} />,
    disabledFor: (s) => (s?.pushAhead ?? 0) === 0,
    invoke: (repo, cb) => cb.onPush(false, repo),
  },
  {
    key: "pr",
    labelKey: "integrations:createPr",
    icon: <IconGitPullRequest className={ICON_CLASS} />,
    disabledFor: () => false,
    invoke: (repo, cb) => cb.onPR(repo),
  },
  {
    key: "pull",
    labelKey: "integrations:pull",
    icon: <IconCloudDownload className={ICON_CLASS} />,
    disabledFor: () => false,
    invoke: (repo, cb) => cb.onPull(repo),
  },
  {
    key: "rebase",
    labelKey: "integrations:rebase",
    icon: <IconGitCherryPick className={ICON_CLASS} />,
    disabledFor: () => false,
    invoke: (repo, cb) => cb.onRebase(repo),
  },
  {
    key: "merge",
    labelKey: "integrations:merge",
    icon: <IconGitMerge className={ICON_CLASS} />,
    disabledFor: () => false,
    invoke: (repo, cb) => cb.onMerge(repo),
  },
  {
    key: "force-push",
    labelKey: "integrations:forcePush",
    icon: <IconAlertTriangle className={ICON_CLASS} />,
    disabledFor: (s) => (s?.pushAhead ?? 0) === 0,
    invoke: (repo, cb) => cb.onPush(true, repo),
  },
];

/**
 * Sub-menu listing every repo for one git action. Per-repo entries show the
 * repo name plus its ahead/behind indicators so the user can pick the right
 * target without leaving the menu. Disabled when the action is a no-op for
 * that repo (e.g. Push when ahead == 0).
 */
function PerRepoActionSub({
  action,
  disabled,
  pushDisabled,
  pullDisabled,
  blockedRepositoryName,
  repoNames,
  perRepoStatus,
  repoDisplayName,
  callbacks,
}: {
  action: ActionDef;
  disabled: boolean;
  pushDisabled: boolean;
  pullDisabled: boolean;
  blockedRepositoryName: string | undefined;
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  repoDisplayName: (repositoryName: string) => string | undefined;
  callbacks: PerRepoCallbacks;
}) {
  const { t } = useTranslation();
  const statusByName = new Map(perRepoStatus.map((s) => [s.repository_name, s]));
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger className={ITEM_CLASS} disabled={disabled}>
        {action.icon}
        <span className="flex-1">{t(action.labelKey)}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-52">
        {repoNames.map((repo) => {
          const status = statusByName.get(repo);
          const ahead = status?.pushAhead ?? 0;
          const behind = status?.pullBehind ?? 0;
          const label = repoDisplayName(repo) || repo || t("integrations:repository");
          const actionBlocked = isRemoteActionBlockedForRepository(
            action.key,
            repo,
            blockedRepositoryName,
            pushDisabled,
            pullDisabled,
          );
          const blockedTitle = actionBlocked
            ? t("task:remoteActionsDisabledHistoryChanged")
            : undefined;
          return (
            <DropdownMenuItem
              key={repo || "__no_repo__"}
              className={ITEM_CLASS}
              onClick={() => action.invoke(repo, callbacks)}
              disabled={disabled || actionBlocked || action.disabledFor(status)}
              title={blockedTitle}
            >
              <span className="flex-1 truncate">{label}</span>
              <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                {ahead > 0 && <span>↑{ahead}</span>}
                {behind > 0 && <span className="text-yellow-500">↓{behind}</span>}
              </span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

/**
 * Action-first dropdown for multi-repo workspaces. Top level lists each git
 * action (Commit / Push / PR / Pull / Rebase / Merge / Force Push); each
 * action expands to a sub-menu of per-repo entries with ahead/behind
 * indicators. Top-level stays a constant 7 items regardless of repo count
 * so the menu doesn't grow unbounded with N repos (the previous repo-first
 * layout was 7 × N items, which scrolled off-screen at 3+ repos).
 */
function MultiRepoVcsDropdown({
  disabled,
  baseBranch,
  repoNames,
  perRepoStatus,
  pushDisabled,
  pullDisabled,
  blockedRepositoryName,
  repoDisplayName,
  callbacks,
}: {
  disabled: boolean;
  baseBranch: string;
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  pushDisabled: boolean;
  pullDisabled: boolean;
  blockedRepositoryName: string | undefined;
  repoDisplayName: (repositoryName: string) => string | undefined;
  callbacks: PerRepoCallbacks;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuContent align="end" className="w-56">
      <DropdownMenuLabel className="text-[10px] text-muted-foreground/70 uppercase tracking-wide">
        {t("integrations:pickActionThenRepo")}
      </DropdownMenuLabel>
      {ACTION_DEFS.map((action, idx) => (
        <div key={action.key}>
          {/* Force Push is the destructive variant of Push; group it visually
              by separating it from the everyday actions above. */}
          {action.key === "force-push" && idx > 0 && <DropdownMenuSeparator />}
          <PerRepoActionSub
            action={action}
            disabled={disabled}
            pushDisabled={pushDisabled}
            pullDisabled={pullDisabled}
            blockedRepositoryName={blockedRepositoryName}
            repoNames={repoNames}
            perRepoStatus={perRepoStatus}
            repoDisplayName={repoDisplayName}
            callbacks={callbacks}
          />
          {action.key === "rebase" && <RebaseMergeFootnote target={baseBranch} type="onto" />}
          {action.key === "merge" && <RebaseMergeFootnote target={baseBranch} type="from" />}
        </div>
      ))}
    </DropdownMenuContent>
  );
}

/** Inline footnote under Rebase/Merge showing the target branch. */
// `type` stays a discriminant and never reaches the screen: rendering the union
// member directly would make "onto"/"from" untranslatable and couple the copy to
// the logic. Each branch resolves a whole message with the ref interpolated.
function RebaseMergeFootnote({ target, type }: { target: string; type: "onto" | "from" }) {
  const { t } = useTranslation();
  return (
    <div className="px-3 py-0.5 text-[10px] text-muted-foreground/60">
      {type === "onto"
        ? t("integrations:ontoBranch", { branch: target })
        : t("integrations:fromBranch", { branch: target })}
    </div>
  );
}

/**
 * Multi-repo top-bar button. The primary button click opens the per-repo
 * dropdown directly (no separate chevron) since every action needs a repo
 * scope. The label / badge / tooltip mirror the single-repo strongest action
 * (Commit / Push / Create PR / Rebase) so the visual CTA is preserved.
 */
export function MultiRepoVcsButton({
  primaryButtonConfig,
  primaryAction,
  isDisabled,
  isGitLoading,
  baseBranch,
  repoNames,
  perRepoStatus,
  pushDisabled,
  pullDisabled,
  blockedRepositoryName,
  repoDisplayName,
  callbacks,
}: {
  primaryButtonConfig: PrimaryButtonConfig;
  primaryAction: "commit" | "push" | "pr" | "rebase";
  isDisabled: boolean;
  isGitLoading: boolean;
  baseBranch: string;
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  pushDisabled: boolean;
  pullDisabled: boolean;
  blockedRepositoryName?: string;
  repoDisplayName: (repositoryName: string) => string | undefined;
  callbacks: PerRepoCallbacks;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="cursor-pointer gap-1"
                disabled={isDisabled}
                data-testid={`vcs-primary-${primaryAction}`}
              >
                {primaryButtonConfig.icon}
                {primaryButtonConfig.label}
                {primaryButtonConfig.badge != null && (
                  <span className="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                    {primaryButtonConfig.badge}
                  </span>
                )}
                {isGitLoading ? (
                  <IconLoader2 className="h-3.5 w-3.5 animate-spin ml-1" />
                ) : (
                  <IconChevronDown className="h-3.5 w-3.5 ml-1" />
                )}
              </Button>
            </DropdownMenuTrigger>
            <MultiRepoVcsDropdown
              disabled={isDisabled}
              baseBranch={baseBranch}
              repoNames={repoNames}
              perRepoStatus={perRepoStatus}
              pushDisabled={pushDisabled}
              pullDisabled={pullDisabled}
              blockedRepositoryName={blockedRepositoryName}
              repoDisplayName={repoDisplayName}
              callbacks={callbacks}
            />
          </DropdownMenu>
        </span>
      </TooltipTrigger>
      <TooltipContent>{t(PICK_REPOSITORY_KEYS[primaryAction])}</TooltipContent>
    </Tooltip>
  );
}
