import { IconAlertTriangle, IconLock } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import type { TaskState } from "@/lib/types/http";
import type { TaskGraphNode } from "@/lib/task-graph/model";
import type { TaskGraphNodeRenderState } from "./task-graph-renderer";

function BlockedLine({ node }: { node: TaskGraphNode }) {
  const { t } = useTranslation();
  if (!node.blocked) return null;
  if (node.blocked_reason === "failed") {
    return (
      <span className="flex items-center gap-1 text-red-600 dark:text-red-400">
        <IconAlertTriangle aria-hidden="true" className="size-3 shrink-0" />
        {t("task:taskGraphPredecessorFailed")}
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1 text-primary">
      <IconLock aria-hidden="true" className="size-3 shrink-0" />
      {t("task:taskGraphBlocked")}
    </span>
  );
}

function Tags({ node }: { node: TaskGraphNode }) {
  const { t } = useTranslation();
  const tags = [
    node.focused && t("task:taskGraphThisTask"),
    node.role === "external" && t("task:taskGraphExternal"),
    node.archived && t("task:taskGraphArchived"),
  ].filter(Boolean) as string[];
  if (tags.length === 0) return null;
  return (
    <span className="flex flex-wrap gap-1">
      {tags.map((tag) => (
        <span
          key={tag}
          className="rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground"
        >
          {tag}
        </span>
      ))}
    </span>
  );
}

/** Renderer-agnostic node body; sized by its container. */
export function TaskGraphNodeCard({
  node,
  state,
}: {
  node: TaskGraphNode;
  state: TaskGraphNodeRenderState;
}) {
  const { t } = useTranslation();
  const taskState = node.state as TaskState;
  return (
    <div
      data-testid="task-graph-node-card"
      data-focused={node.focused}
      data-dimmed={state.dimmed}
      data-emphasized={state.emphasized}
      data-selected={state.selected}
      className={cn(
        "flex h-full w-full flex-col gap-1 overflow-hidden rounded-md border bg-card p-2 text-left text-xs text-card-foreground shadow-sm transition-opacity",
        node.role === "external" && "border-dashed bg-muted/40",
        node.archived && "opacity-60",
        node.focused && "ring-2 ring-primary",
        state.selected && "ring-2 ring-primary ring-offset-2 ring-offset-background",
        state.dimmed && "opacity-30",
      )}
    >
      <div className="flex min-w-0 items-start gap-1.5">
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[10px] font-semibold text-primary">
          {node.step}
        </span>
        <span className="line-clamp-2 min-w-0 flex-1 font-medium leading-snug">{node.title}</span>
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-muted-foreground">
        <span className="flex items-center gap-1">
          {getTaskStateIcon(taskState, "size-3 shrink-0")}
          {formatTaskStateLabel(taskState)}
        </span>
        <BlockedLine node={node} />
        {node.depth > 1 && node.parent_title && (
          <span className="truncate">
            {t("task:taskGraphInParent", { title: node.parent_title })}
          </span>
        )}
      </div>
      <Tags node={node} />
    </div>
  );
}
