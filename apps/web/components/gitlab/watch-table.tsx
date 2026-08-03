"use client";

import {
  IconPlayerPause,
  IconPlayerPlay,
  IconEdit,
  IconRefresh,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { IssueWatch, ReviewWatch } from "@/lib/types/gitlab";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatDateTime } from "@/lib/i18n/formats";

type Watch = ReviewWatch | IssueWatch;

export type WatchTableProps<TWatch extends Watch> = {
  watches: TWatch[];
  dirtyIds: ReadonlySet<string>;
  authoritativeEnabledById: ReadonlyMap<string, boolean>;
  onEdit: (watch: TWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onToggleEnabled: (watch: TWatch) => void;
};

type ActionProps<TWatch extends Watch> = Omit<WatchTableProps<TWatch>, "watches"> & {
  watch: TWatch;
  mobile?: boolean;
};

// `t` is threaded in rather than read from a hook: these are plain functions, and
// the guard never inspects a return value, so a literal here would survive lint.
function checkUnavailableReason(
  t: TFunction,
  dirty: boolean,
  enabled: boolean,
): string | undefined {
  if (dirty) return t("gitlab:saveChangesBeforeCheckingNow");
  if (!enabled) return t("gitlab:enableThisWatchBeforeCheckingNow");
  return undefined;
}

function CheckNowButton({
  size,
  disabledReason,
  onClick,
}: {
  size: string;
  disabledReason?: string;
  onClick: (event: React.MouseEvent) => void;
}) {
  const { t } = useTranslation();
  const button = (
    <Button
      variant="ghost"
      size="sm"
      className={`${size} p-0 cursor-pointer`}
      aria-label={t("gitlab:checkNow")}
      aria-description={disabledReason}
      title={disabledReason ? undefined : t("gitlab:checkNow")}
      disabled={Boolean(disabledReason)}
      onClick={onClick}
    >
      <IconRefresh className="h-4 w-4" />
    </Button>
  );
  if (!disabledReason) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} aria-label={disabledReason} className={`inline-flex ${size}`}>
          {button}
        </span>
      </TooltipTrigger>
      <TooltipContent>{disabledReason}</TooltipContent>
    </Tooltip>
  );
}

function WatchActions<TWatch extends Watch>({
  watch,
  dirtyIds,
  authoritativeEnabledById,
  onEdit,
  onDelete,
  onTrigger,
  onReset,
  onToggleEnabled,
  mobile,
}: ActionProps<TWatch>) {
  const { t } = useTranslation();
  const size = mobile ? "h-11 w-11" : "h-8 w-8";
  const dirty = dirtyIds.has(watch.id);
  const authoritativeEnabled = authoritativeEnabledById.get(watch.id) ?? watch.enabled;
  const checkReason = checkUnavailableReason(t, dirty, authoritativeEnabled);
  const stop = (action: () => void) => (event: React.MouseEvent) => {
    event.stopPropagation();
    action();
  };
  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 cursor-pointer`}
        aria-label={t("gitlab:editWatch")}
        onClick={stop(() => onEdit(watch))}
      >
        <IconEdit className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 cursor-pointer`}
        aria-label={watch.enabled ? t("gitlab:pauseWatch") : t("gitlab:enableWatch")}
        data-settings-dirty={dirty}
        onClick={stop(() => onToggleEnabled(watch))}
      >
        {watch.enabled ? (
          <IconPlayerPause className="h-4 w-4" />
        ) : (
          <IconPlayerPlay className="h-4 w-4" />
        )}
      </Button>
      <CheckNowButton
        size={size}
        disabledReason={checkReason}
        onClick={stop(() => onTrigger(watch.id))}
      />
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 cursor-pointer`}
        aria-label={t("gitlab:resetWatch")}
        onClick={stop(() => onReset(watch.id))}
      >
        <IconRestore className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 text-destructive hover:text-destructive cursor-pointer`}
        aria-label={t("gitlab:deleteWatch")}
        onClick={stop(() => onDelete(watch.id))}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}

// Project paths are user data (namespace/project as GitLab stores it) — only the
// empty-selection fallback is copy.
function projectSummary(t: TFunction, watch: Watch): string {
  return watch.projects.length > 0
    ? watch.projects.map((project) => project.path).join(", ")
    : t("gitlab:allProjects");
}

// `formatDateTime` follows the active app locale; the previous bare
// `toLocaleString()` followed the browser's, so the timestamp could disagree with
// the rest of the page after a language switch.
function lastPolled(t: TFunction, value?: string): string {
  if (!value) return t("gitlab:never");
  return formatDateTime(value);
}

function WatchError({ watch }: { watch: Watch }) {
  if (!watch.last_error) return null;
  return (
    <p className="mt-1 break-words text-xs text-destructive" role="status">
      {watch.last_error}
    </p>
  );
}

function MobileWatchCard<TWatch extends Watch>(props: ActionProps<TWatch>) {
  const { t } = useTranslation();
  const { watch, onEdit } = props;
  return (
    <div
      className="space-y-3 border-b px-1 py-4 last:border-b-0"
      data-settings-dirty={props.dirtyIds.has(watch.id)}
    >
      <button
        type="button"
        className="block w-full min-w-0 text-left cursor-pointer"
        onClick={() => onEdit(watch)}
      >
        <span className="block truncate text-sm font-medium" title={projectSummary(t, watch)}>
          {projectSummary(t, watch)}
        </span>
        <span className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={watch.enabled ? "default" : "secondary"}>
            {watch.enabled ? t("gitlab:active") : t("gitlab:paused")}
          </Badge>
          <span>
            {t("gitlab:intervalMinutesLabel", {
              count: Math.round(watch.poll_interval_seconds / 60),
            })}
          </span>
          <span>{t("gitlab:lastCheckedAt", { value: lastPolled(t, watch.last_polled_at) })}</span>
        </span>
        <WatchError watch={watch} />
      </button>
      <WatchActions {...props} mobile />
    </div>
  );
}

export function GitLabWatchTable<TWatch extends Watch>(props: WatchTableProps<TWatch>) {
  const { t } = useTranslation();
  if (props.watches.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t("gitlab:noWatchesConfigured")}
      </p>
    );
  }
  return (
    <>
      <div className="md:hidden" data-testid="gitlab-watch-mobile-list">
        {props.watches.map((watch) => (
          <MobileWatchCard key={watch.id} {...props} watch={watch} />
        ))}
      </div>
      <div
        className="hidden max-w-full overflow-x-auto md:block"
        data-testid="gitlab-watch-desktop-table"
      >
        <Table className="min-w-[680px]">
          <TableHeader>
            <TableRow>
              <TableHead>{t("gitlab:projects")}</TableHead>
              <TableHead>{t("gitlab:interval")}</TableHead>
              <TableHead>{t("gitlab:lastChecked")}</TableHead>
              <TableHead>{t("common:status")}</TableHead>
              <TableHead className="text-right">{t("gitlab:actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.watches.map((watch) => (
              <TableRow
                key={watch.id}
                className="cursor-pointer"
                data-settings-dirty={props.dirtyIds.has(watch.id)}
                onClick={() => props.onEdit(watch)}
              >
                <TableCell className="max-w-sm">
                  <p className="truncate font-medium" title={projectSummary(t, watch)}>
                    {projectSummary(t, watch)}
                  </p>
                  <WatchError watch={watch} />
                </TableCell>
                <TableCell>
                  {t("gitlab:intervalMinutes", {
                    count: Math.round(watch.poll_interval_seconds / 60),
                  })}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {lastPolled(t, watch.last_polled_at)}
                </TableCell>
                <TableCell>
                  <Badge variant={watch.enabled ? "default" : "secondary"}>
                    {watch.enabled ? t("gitlab:active") : t("gitlab:paused")}
                  </Badge>
                </TableCell>
                <TableCell>
                  <WatchActions {...props} watch={watch} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}
