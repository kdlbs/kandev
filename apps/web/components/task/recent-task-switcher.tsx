"use client";

import { IconHistory, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Kbd } from "@kandev/ui/kbd";
import { cn } from "@/lib/utils";
import {
  useRecentTaskSwitcherController,
  type RecentTaskSwitcherController,
} from "./recent-task-switcher-hooks";
import type { RecentTaskDisplayItem } from "./recent-task-switcher-model";

function TaskBadges({ item }: { item: RecentTaskDisplayItem }) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      <Badge
        variant={item.statusBadge.variant}
        data-testid="recent-task-switcher-badge-status"
        className="max-w-full"
      >
        {item.statusBadge.label}
      </Badge>
      {item.repositoryPath && (
        <Badge
          variant="outline"
          data-testid="recent-task-switcher-badge-repository"
          className="max-w-[11rem] truncate"
        >
          {item.repositoryPath}
        </Badge>
      )}
      {item.workflowName && (
        <Badge
          variant="secondary"
          data-testid="recent-task-switcher-badge-workflow"
          className="max-w-[11rem] truncate"
        >
          {item.workflowName}
        </Badge>
      )}
      {item.workflowStepTitle && (
        <Badge variant="outline" className="max-w-[9rem] truncate">
          {item.workflowStepTitle}
        </Badge>
      )}
      {item.isCurrent && (
        <Badge variant="outline" data-testid="recent-task-switcher-badge-current">
          Current
        </Badge>
      )}
    </div>
  );
}

function RecentTaskRow({
  item,
  selected,
  onSelect,
  onHover,
}: {
  item: RecentTaskDisplayItem;
  selected: boolean;
  onSelect: () => void;
  onHover: () => void;
}) {
  return (
    <button
      type="button"
      data-testid={`recent-task-switcher-item-${item.taskId}`}
      data-selected={selected ? "true" : "false"}
      onClick={onSelect}
      onMouseEnter={onHover}
      className={cn(
        "grid w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-md border px-3 py-2.5 text-left transition-colors",
        selected
          ? "border-primary/70"
          : "border-transparent hover:border-border hover:bg-accent/40",
      )}
    >
      <span className="min-w-0 space-y-1.5">
        <span className="block truncate text-sm font-medium leading-none">{item.title}</span>
        <TaskBadges item={item} />
      </span>
      <span className={cn("size-2 rounded-full", selected ? "bg-primary" : "bg-border")} />
    </button>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
      <IconHistory className="size-6 text-muted-foreground/60" />
      <p className="text-sm text-muted-foreground">No recent tasks yet</p>
    </div>
  );
}

function RecentTaskList({
  items,
  selectedIndex,
  setSelectedIndex,
  selectItem,
}: Pick<
  RecentTaskSwitcherController,
  "items" | "selectedIndex" | "setSelectedIndex" | "selectItem"
>) {
  if (items.length === 0) return <EmptyState />;

  return (
    <div className="space-y-1">
      {items.map((item, index) => (
        <RecentTaskRow
          key={item.taskId}
          item={item}
          selected={index === selectedIndex}
          onSelect={() => selectItem(item)}
          onHover={() => setSelectedIndex(index)}
        />
      ))}
    </div>
  );
}

function SwitcherFooter({ shortcutLabel }: { shortcutLabel: string }) {
  return (
    <div className="border-t px-4 py-2 text-[11px] text-muted-foreground">
      <Kbd>{shortcutLabel}</Kbd>
    </div>
  );
}

function RecentTaskSwitcherDialog(props: RecentTaskSwitcherController) {
  return (
    <Dialog open={props.open} onOpenChange={props.setOpen}>
      <DialogContent
        data-testid="recent-task-switcher"
        className="max-w-[min(42rem,calc(100vw-2rem))] gap-0 overflow-hidden p-0"
        showCloseButton={false}
        onKeyDown={props.handleKeyDown}
      >
        <DialogHeader className="border-b px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-sm">
            <IconRefresh className="size-4 text-muted-foreground" />
            Recent Tasks
          </DialogTitle>
          <DialogDescription className="sr-only">Switch recent tasks.</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto p-2">
          <RecentTaskList
            items={props.items}
            selectedIndex={props.selectedIndex}
            setSelectedIndex={props.setSelectedIndex}
            selectItem={props.selectItem}
          />
        </div>
        <SwitcherFooter shortcutLabel={props.shortcutLabel} />
      </DialogContent>
    </Dialog>
  );
}

export function RecentTaskSwitcher() {
  return <RecentTaskSwitcherDialog {...useRecentTaskSwitcherController()} />;
}
