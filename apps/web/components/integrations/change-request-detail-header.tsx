import { IconGitMerge, IconMinus, IconPlus, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatTimeAgo, getTimeAgoColor } from "@/components/github/pr-shared";
import { t } from "@/lib/i18n";
import { ChangeRequestDetailCopyButton } from "./change-request-detail-copy-button";
import type {
  ChangeRequestDetailAction,
  ChangeRequestDetailModel,
  ChangeRequestDetailPerson,
  ChangeRequestDetailProps,
} from "./change-request-detail";

export function ChangeRequestPersonLink({ person }: { person: ChangeRequestDetailPerson }) {
  const content = <span className="truncate">{person.name}</span>;
  if (!person.url) return <span className="text-xs font-medium">{content}</span>;
  return (
    <a
      href={person.url}
      target="_blank"
      rel="noopener noreferrer"
      className="min-w-0 max-w-full truncate text-xs font-medium hover:underline"
    >
      {content}
    </a>
  );
}

function actionVariant(tone: ChangeRequestDetailAction["tone"]) {
  if (tone === "danger") return "destructive" as const;
  if (tone === "secondary") return "outline" as const;
  return "default" as const;
}

export function ChangeRequestActionButton({
  action,
  busy,
  onAction,
  ariaLabel,
  testId,
  type = "button",
}: {
  action: ChangeRequestDetailAction;
  busy: boolean;
  onAction: () => void;
  ariaLabel?: string;
  testId?: string;
  type?: "button" | "submit";
}) {
  const successClass =
    action.tone === "success" ? "border-0 bg-green-600 text-white hover:bg-green-700" : "";
  return (
    <Button
      type={type}
      size="sm"
      variant={actionVariant(action.tone)}
      className={`min-h-11 cursor-pointer sm:min-h-0 ${successClass}`}
      disabled={action.disabled || busy}
      onClick={onAction}
      aria-label={ariaLabel}
      data-testid={testId}
    >
      {busy ? (action.pendingLabel ?? action.label) : action.label}
    </Button>
  );
}

function StateBadge({ state }: { state: string }) {
  const styles: Record<string, string> = {
    open: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    draft: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
    merged: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
    closed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    declined: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  };
  return (
    <Badge variant="secondary" className={`px-1.5 py-0 text-[10px] ${styles[state] ?? ""}`}>
      {state}
    </Badge>
  );
}

function HeaderDates({ detail }: { detail: ChangeRequestDetailModel }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
      <span className="flex items-center gap-0.5">
        {t("integrations:by")} <ChangeRequestPersonLink person={detail.author} />
      </span>
      {detail.createdAt ? (
        <>
          <span>&middot;</span>
          <span className={getTimeAgoColor(detail.createdAt)}>
            {t("integrations:openedRelative", { relative: formatTimeAgo(detail.createdAt) })}
          </span>
        </>
      ) : null}
      {detail.mergedAt ? (
        <>
          <span>&middot;</span>
          <span className="flex items-center gap-0.5">
            <IconGitMerge className="h-3 w-3 text-purple-500" />
            {t("integrations:mergedRelative", { relative: formatTimeAgo(detail.mergedAt) })}
          </span>
        </>
      ) : null}
      {detail.closedAt && !detail.mergedAt ? (
        <>
          <span>&middot;</span>
          <span>
            {t("integrations:closedRelative", { relative: formatTimeAgo(detail.closedAt) })}
          </span>
        </>
      ) : null}
    </div>
  );
}

function HeaderStats({ detail }: { detail: ChangeRequestDetailModel }) {
  const reviewCount = detail.reviews.length;
  const pendingCount = detail.requestedReviewers.length || detail.pendingReviewCount || 0;
  return (
    <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
      <span className="flex items-center gap-1">
        <IconPlus className="h-3 w-3 text-green-500" />
        {detail.additions}
      </span>
      <span className="flex items-center gap-1">
        <IconMinus className="h-3 w-3 text-red-500" />
        {detail.deletions}
      </span>
      <span>&middot;</span>
      <span>
        {t("integrations:reviewCount", { count: reviewCount })}
        {pendingCount ? (
          <span className="text-yellow-600 dark:text-yellow-400">
            {` (${t("integrations:pendingCount", { count: pendingCount })})`}
          </span>
        ) : null}
      </span>
      <span>&middot;</span>
      <span>{t("integrations:commentCount", { count: detail.comments.length })}</span>
      {detail.reviewState ? (
        <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
          {detail.reviewState.replaceAll("_", " ")}
        </Badge>
      ) : null}
    </div>
  );
}

export function ChangeRequestDetailHeader({
  props,
  detail,
}: {
  props: ChangeRequestDetailProps;
  detail: ChangeRequestDetailModel;
}) {
  const actions = props.actions?.filter((action) => action.placement === "header") ?? [];
  const state = detail.draft && detail.state === "open" ? "draft" : detail.state;
  return (
    <div className="space-y-2 p-3">
      <div className="flex flex-wrap items-start gap-2">
        <a
          data-testid="change-request-detail-title"
          href={detail.url}
          target="_blank"
          rel="noopener noreferrer"
          className="min-w-0 grow basis-auto break-words text-sm font-medium hover:underline"
        >
          {detail.title}
        </a>
        <div
          data-testid="change-request-detail-actions"
          className="flex min-w-0 max-w-full shrink flex-wrap justify-start gap-1.5"
        >
          {actions.map((action) => (
            <ChangeRequestActionButton
              key={action.id}
              action={action}
              busy={props.busyActionId === action.id}
              onAction={() => void props.onAction?.({ actionId: action.id })}
            />
          ))}
          {props.headerActions ? (
            <div className="contents [&_button]:min-h-11 sm:[&_button]:min-h-0">
              {props.headerActions}
            </div>
          ) : null}
          {props.onRefresh ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="h-11 w-11 cursor-pointer p-0 text-muted-foreground sm:h-6 sm:w-6"
                  onClick={props.onRefresh}
                  disabled={props.loading}
                  aria-label={t("integrations:refresh")}
                >
                  <IconRefresh className={`h-3.5 w-3.5 ${props.loading ? "animate-spin" : ""}`} />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("integrations:refresh")}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <StateBadge state={state} />
        <span className="text-xs text-muted-foreground">#{detail.number}</span>
        <ChangeRequestDetailCopyButton
          url={detail.url}
          kind="changeRequest"
          testId="change-request-copy-url"
        />
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[10px]">
          {detail.sourceBranch}
        </code>
        <span className="mx-0.5 text-muted-foreground">&rarr;</span>
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[10px]">
          {detail.targetBranch}
        </code>
      </div>
      {props.notice}
      <HeaderDates detail={detail} />
      <HeaderStats detail={detail} />
    </div>
  );
}
