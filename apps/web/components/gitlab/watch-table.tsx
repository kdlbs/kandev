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

function checkUnavailableReason(dirty: boolean, enabled: boolean): string | undefined {
  if (dirty) return "Save changes before checking now";
  if (!enabled) return "Enable this watch before checking now";
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
  const button = (
    <Button
      variant="ghost"
      size="sm"
      className={`${size} p-0 cursor-pointer`}
      aria-label="Check now"
      aria-description={disabledReason}
      title={disabledReason ? undefined : "Check now"}
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
  const size = mobile ? "h-11 w-11" : "h-8 w-8";
  const dirty = dirtyIds.has(watch.id);
  const authoritativeEnabled = authoritativeEnabledById.get(watch.id) ?? watch.enabled;
  const checkReason = checkUnavailableReason(dirty, authoritativeEnabled);
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
        aria-label="Edit watch"
        onClick={stop(() => onEdit(watch))}
      >
        <IconEdit className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 cursor-pointer`}
        aria-label={watch.enabled ? "Pause watch" : "Enable watch"}
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
        aria-label="Reset watch"
        onClick={stop(() => onReset(watch.id))}
      >
        <IconRestore className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className={`${size} p-0 text-destructive hover:text-destructive cursor-pointer`}
        aria-label="Delete watch"
        onClick={stop(() => onDelete(watch.id))}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}

function projectSummary(watch: Watch): string {
  return watch.projects.length > 0
    ? watch.projects.map((project) => project.path).join(", ")
    : "All projects";
}

function lastPolled(value?: string): string {
  if (!value) return "Never";
  return new Date(value).toLocaleString();
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
        <span className="block truncate text-sm font-medium" title={projectSummary(watch)}>
          {projectSummary(watch)}
        </span>
        <span className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={watch.enabled ? "default" : "secondary"}>
            {watch.enabled ? "Active" : "Paused"}
          </Badge>
          <span>{Math.round(watch.poll_interval_seconds / 60)}m interval</span>
          <span>Last checked {lastPolled(watch.last_polled_at)}</span>
        </span>
        <WatchError watch={watch} />
      </button>
      <WatchActions {...props} mobile />
    </div>
  );
}

export function GitLabWatchTable<TWatch extends Watch>(props: WatchTableProps<TWatch>) {
  if (props.watches.length === 0) {
    return <p className="py-6 text-center text-sm text-muted-foreground">No watches configured.</p>;
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
              <TableHead>Projects</TableHead>
              <TableHead>Interval</TableHead>
              <TableHead>Last checked</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Actions</TableHead>
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
                  <p className="truncate font-medium" title={projectSummary(watch)}>
                    {projectSummary(watch)}
                  </p>
                  <WatchError watch={watch} />
                </TableCell>
                <TableCell>{Math.round(watch.poll_interval_seconds / 60)}m</TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {lastPolled(watch.last_polled_at)}
                </TableCell>
                <TableCell>
                  <Badge variant={watch.enabled ? "default" : "secondary"}>
                    {watch.enabled ? "Active" : "Paused"}
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
