import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import TaskLink from "@/components/routing/task-link";
import { formatDateTime, formatRelative } from "@/lib/i18n/formats";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";
import type { UndoMessage } from "@/hooks/domains/coordinator/activity-controller";
import type { PersonName } from "@/hooks/domains/coordinator/use-activity-members";
import {
  actionText,
  authorizationLine,
  classLabel,
  MESSAGE_TEXT_KEY,
  outcomeLine,
  undoneText,
} from "./activity-text";

export type TaskAvailability = {
  /** The snapshots hold this task. */
  held: boolean;
  /** The snapshots have loaded without error at least once. */
  trusted: boolean;
};

export type WhatItDidRowProps = {
  item: ActivityItem;
  now: number;
  canManage: boolean;
  busy: boolean;
  message: UndoMessage | undefined;
  resolvePerson: (userId: string | null) => PersonName;
  taskAvailability: TaskAvailability;
  onUndo: (item: ActivityItem) => void;
};

function WhenCell({ item, now }: { item: ActivityItem; now: number }) {
  const { t } = useTranslation();
  const relative = formatRelative(item.created_at, now);
  if (!relative) return <div data-testid="activity-when" />;
  const repeated = item.refusal_count > 1 && !Number.isNaN(new Date(item.updated_at).getTime());
  return (
    <div data-testid="activity-when" className="text-muted-foreground text-xs">
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={0} className="cursor-default">
            {relative}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <div>{formatDateTime(item.created_at)}</div>
          {repeated && (
            <div>
              {t("coordinator:activityLastRepeat", { time: formatDateTime(item.updated_at) })}
            </div>
          )}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

function TaskReference({
  item,
  availability,
}: {
  item: ActivityItem;
  availability: TaskAvailability;
}) {
  const { t } = useTranslation();
  const taskId = item.target_task_id;
  if (!taskId) return null;
  if (item.target_task_identifier) {
    return (
      <TaskLink taskId={taskId} className="text-primary cursor-pointer hover:underline">
        {item.target_task_identifier}
      </TaskLink>
    );
  }
  if (availability.held) {
    return (
      <TaskLink taskId={taskId} className="text-primary cursor-pointer hover:underline">
        {t("coordinator:activityOpenTask")}
      </TaskLink>
    );
  }
  if (availability.trusted) {
    return <span className="text-muted-foreground">{t("coordinator:activityTaskGone")}</span>;
  }
  return null;
}

function ActionCell({
  item,
  availability,
}: {
  item: ActivityItem;
  availability: TaskAvailability;
}) {
  const { t } = useTranslation();
  const text = actionText(item, t);
  return (
    <div className="min-w-0 text-sm" data-testid="activity-action">
      <span className="line-clamp-2 inline" title={text}>
        {text}
      </span>{" "}
      <TaskReference item={item} availability={availability} />
      {item.unattended_turn_id && (
        <div className="text-muted-foreground text-xs" data-testid="activity-unattended">
          {t("coordinator:activityUnattended")}
        </div>
      )}
    </div>
  );
}

function AuthorizationCell({
  item,
  resolvePerson,
}: {
  item: ActivityItem;
  resolvePerson: (userId: string | null) => PersonName;
}) {
  const { t } = useTranslation();
  const line = authorizationLine(item, t);
  const outcome = outcomeLine(item, resolvePerson(item.actor_user_id), t);
  return (
    <div className="text-xs" data-testid="activity-authorization">
      {line && <div>{line}</div>}
      {outcome && <div className="text-muted-foreground">{outcome}</div>}
    </div>
  );
}

type UndoCellProps = Pick<
  WhatItDidRowProps,
  "item" | "now" | "canManage" | "busy" | "message" | "resolvePerson" | "onUndo"
>;

function UndoControl({ item, now, canManage, busy, resolvePerson, onUndo }: UndoCellProps) {
  const { t } = useTranslation();
  if (item.undone_at) {
    return (
      <span className="text-muted-foreground text-xs">
        {undoneText(resolvePerson(item.undone_by), formatRelative(item.undone_at, now), t)}
      </span>
    );
  }
  if (item.undoable && canManage) {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="w-full cursor-pointer aria-disabled:cursor-default aria-disabled:opacity-50 md:w-auto"
        aria-disabled={busy}
        data-testid={`activity-undo-${item.id}`}
        onClick={() => {
          if (!busy) onUndo(item);
        }}
      >
        {t("coordinator:activityUndoAction")}
      </Button>
    );
  }
  if (
    item.action_class === "message" ||
    item.action_class === "resume" ||
    item.outcome === "returned"
  ) {
    return <span className="text-muted-foreground text-xs">{t("coordinator:activityNoUndo")}</span>;
  }
  return null;
}

function UndoCell(props: UndoCellProps) {
  const { t } = useTranslation();
  const { message } = props;
  return (
    <div className="flex flex-col gap-1 text-xs" data-testid="activity-undo-cell">
      <UndoControl {...props} />
      {message && (
        <div role="status" className="text-muted-foreground" data-testid="activity-undo-message">
          {t(MESSAGE_TEXT_KEY[message.key])}
        </div>
      )}
    </div>
  );
}

export const ACTIVITY_GRID = "md:grid-cols-[6rem_minmax(0,1fr)_6.5rem_11rem_8rem]";

export function WhatItDidRow(props: WhatItDidRowProps) {
  const { t } = useTranslation();
  const { item } = props;
  return (
    <div
      role="row"
      data-testid={`activity-row-${item.id}`}
      className={`grid grid-cols-2 gap-x-3 gap-y-1 px-3 py-2 md:items-start ${ACTIVITY_GRID}`}
    >
      <div role="cell" className="order-1 md:order-none">
        <WhenCell item={item} now={props.now} />
      </div>
      <div role="cell" className="order-3 col-span-2 md:order-none md:col-span-1">
        <ActionCell item={item} availability={props.taskAvailability} />
      </div>
      <div
        role="cell"
        className="order-2 justify-self-end text-xs md:order-none md:justify-self-start"
        data-testid="activity-class"
      >
        {classLabel(item.action_class, t)}
      </div>
      <div role="cell" className="order-4 col-span-2 md:order-none md:col-span-1">
        <AuthorizationCell item={item} resolvePerson={props.resolvePerson} />
      </div>
      <div role="cell" className="order-5 col-span-2 md:order-none md:col-span-1">
        <UndoCell {...props} />
      </div>
    </div>
  );
}
