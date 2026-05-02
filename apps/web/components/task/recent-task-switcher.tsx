"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import { useRouter } from "next/navigation";
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
import { useAppStore } from "@/components/state-provider";
import { useCommandPanelOpen } from "@/lib/commands/command-registry";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import { linkToTask } from "@/lib/links";
import { cn } from "@/lib/utils";
import {
  getRecentTasks,
  RECENT_TASKS_CHANGED_EVENT,
  RECENT_TASKS_STORAGE_KEY,
  upsertRecentTask,
  type RecentTaskEntry,
} from "@/lib/recent-tasks";
import {
  buildRecentTaskDisplayItems,
  buildRecentTaskEntry,
  getInitialSelectionIndex,
  getNextSelectionIndex,
  getPreviousSelectionIndex,
  type RecentTaskBuildContext,
  type RecentTaskDisplayItem,
} from "./recent-task-switcher-model";
import {
  hasHoldModifier,
  isCommitReleaseEvent,
  isCycleShortcutEvent,
} from "./recent-task-switcher-keys";

function useRecentTaskEntries() {
  const [entries, setEntries] = useState<RecentTaskEntry[]>(() => getRecentTasks());

  useEffect(() => {
    const handleChanged = (event: Event) => {
      const customEvent = event as CustomEvent<{ entries?: RecentTaskEntry[] }>;
      setEntries(customEvent.detail?.entries ?? getRecentTasks());
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key === RECENT_TASKS_STORAGE_KEY) setEntries(getRecentTasks());
    };

    window.addEventListener(RECENT_TASKS_CHANGED_EVENT, handleChanged);
    window.addEventListener("storage", handleStorage);
    return () => {
      window.removeEventListener(RECENT_TASKS_CHANGED_EVENT, handleChanged);
      window.removeEventListener("storage", handleStorage);
    };
  }, []);

  return [entries, setEntries] as const;
}

function useRecentTaskBuildContext(): RecentTaskBuildContext {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const workflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusByEnvId = useAppStore((state) => state.gitStatus.byEnvironmentId);
  const environmentIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);

  return useMemo(
    () => ({
      activeTaskId,
      activeWorkspaceId,
      kanbanWorkflowId,
      kanbanTasks,
      kanbanSteps,
      snapshots,
      workflows,
      repositoriesByWorkspace,
      sessionsByTaskId,
      gitStatusByEnvId,
      environmentIdBySessionId,
    }),
    [
      activeTaskId,
      activeWorkspaceId,
      kanbanWorkflowId,
      kanbanTasks,
      kanbanSteps,
      snapshots,
      workflows,
      repositoriesByWorkspace,
      sessionsByTaskId,
      gitStatusByEnvId,
      environmentIdBySessionId,
    ],
  );
}

function getEntrySignature(entry: RecentTaskEntry): string {
  return JSON.stringify({ ...entry, visitedAt: "" });
}

function useRecordActiveTask(
  context: RecentTaskBuildContext,
  setEntries: (entries: RecentTaskEntry[]) => void,
) {
  const lastTaskIdRef = useRef<string | null>(null);
  const lastSignatureRef = useRef<string | null>(null);

  useEffect(() => {
    if (!context.activeTaskId) return;

    const previous = getRecentTasks().find((entry) => entry.taskId === context.activeTaskId);
    const isNewVisit = lastTaskIdRef.current !== context.activeTaskId;
    const visitedAt = isNewVisit ? new Date().toISOString() : undefined;
    const entry = buildRecentTaskEntry(context.activeTaskId, context, previous, visitedAt);
    const signature = getEntrySignature(entry);

    if (!isNewVisit && lastSignatureRef.current === signature) return;

    lastTaskIdRef.current = context.activeTaskId;
    lastSignatureRef.current = signature;
    setEntries(upsertRecentTask(entry));
  }, [context, setEntries]);
}

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

function getResolvedSelectionIndex(
  selectedIndex: number,
  items: RecentTaskDisplayItem[],
  activeTaskId: string | null,
): number {
  if (selectedIndex >= 0 && selectedIndex < items.length) return selectedIndex;
  return getInitialSelectionIndex(items, activeTaskId);
}

function useLatestRef<T>(value: T) {
  const ref = useRef(value);
  useEffect(() => {
    ref.current = value;
  }, [value]);
  return ref;
}

type RecentTaskSwitcherController = {
  open: boolean;
  setOpen: (open: boolean) => void;
  items: RecentTaskDisplayItem[];
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
  shortcutLabel: string;
  selectItem: (item: RecentTaskDisplayItem | undefined) => void;
  handleKeyDown: (event: React.KeyboardEvent) => void;
};

type SwitcherRefs = {
  openRef: MutableRefObject<boolean>;
  selectedIndexRef: MutableRefObject<number>;
  commitOnReleaseRef: MutableRefObject<boolean>;
  cancelledRef: MutableRefObject<boolean>;
  itemsRef: MutableRefObject<RecentTaskDisplayItem[]>;
  activeTaskIdRef: MutableRefObject<string | null>;
  shortcutRef: MutableRefObject<KeyboardShortcut>;
};

type SwitcherActions = {
  setOpen: (open: boolean) => void;
  setSelectedIndex: (index: number) => void;
  selectItem: (item: RecentTaskDisplayItem | undefined) => void;
  openSwitcher: (commitOnRelease: boolean) => void;
  cycleSwitcher: (commitOnRelease: boolean) => void;
  cancelSwitcher: () => void;
  selectCurrentItem: () => void;
};

function useSwitcherRefs(
  open: boolean,
  selectedIndex: number,
  items: RecentTaskDisplayItem[],
  activeTaskId: string | null,
  shortcut: KeyboardShortcut,
): SwitcherRefs {
  const openRef = useRef(open);
  const selectedIndexRef = useRef(selectedIndex);
  const commitOnReleaseRef = useRef(false);
  const cancelledRef = useRef(false);
  const itemsRef = useLatestRef(items);
  const activeTaskIdRef = useLatestRef(activeTaskId);
  const shortcutRef = useLatestRef(shortcut);

  useEffect(() => {
    openRef.current = open;
    selectedIndexRef.current = selectedIndex;
  }, [open, selectedIndex, openRef, selectedIndexRef]);

  return {
    openRef,
    selectedIndexRef,
    commitOnReleaseRef,
    cancelledRef,
    itemsRef,
    activeTaskIdRef,
    shortcutRef,
  };
}

function useSelectedIndexSetter(refs: SwitcherRefs, setRawSelectedIndex: (index: number) => void) {
  const { activeTaskIdRef, itemsRef, selectedIndexRef } = refs;
  return useCallback(
    (index: number) => {
      selectedIndexRef.current = getResolvedSelectionIndex(
        index,
        itemsRef.current,
        activeTaskIdRef.current,
      );
      setRawSelectedIndex(index);
    },
    [activeTaskIdRef, itemsRef, selectedIndexRef, setRawSelectedIndex],
  );
}

function useSwitcherActions({
  refs,
  routeToTask,
  setCommandPanelOpen,
  setOpenState,
  setSelectedIndex,
}: {
  refs: SwitcherRefs;
  routeToTask: (taskId: string) => void;
  setCommandPanelOpen: (open: boolean) => void;
  setOpenState: (open: boolean) => void;
  setSelectedIndex: (index: number) => void;
}): SwitcherActions {
  const { activeTaskIdRef, cancelledRef, commitOnReleaseRef, itemsRef, openRef, selectedIndexRef } =
    refs;

  const closeSwitcher = useCallback(() => {
    openRef.current = false;
    commitOnReleaseRef.current = false;
    setOpenState(false);
  }, [commitOnReleaseRef, openRef, setOpenState]);

  const cancelSwitcher = useCallback(() => {
    cancelledRef.current = true;
    closeSwitcher();
  }, [cancelledRef, closeSwitcher]);

  const setOpen = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        cancelSwitcher();
        return;
      }
      cancelledRef.current = false;
      openRef.current = true;
      setOpenState(true);
    },
    [cancelSwitcher, cancelledRef, openRef, setOpenState],
  );

  const selectItem = useCallback(
    (item: RecentTaskDisplayItem | undefined) => {
      closeSwitcher();
      if (item) routeToTask(item.taskId);
    },
    [closeSwitcher, routeToTask],
  );

  const openSwitcher = useCallback(
    (commitOnRelease: boolean) => {
      setCommandPanelOpen(false);
      cancelledRef.current = false;
      commitOnReleaseRef.current = commitOnRelease;
      openRef.current = true;
      setSelectedIndex(getInitialSelectionIndex(itemsRef.current, activeTaskIdRef.current));
      setOpenState(true);
    },
    [
      activeTaskIdRef,
      cancelledRef,
      commitOnReleaseRef,
      itemsRef,
      openRef,
      setCommandPanelOpen,
      setOpenState,
      setSelectedIndex,
    ],
  );

  const cycleSwitcher = useCallback(
    (commitOnRelease: boolean) => {
      if (!openRef.current) {
        openSwitcher(commitOnRelease);
        return;
      }
      if (commitOnRelease) commitOnReleaseRef.current = true;
      setSelectedIndex(getNextSelectionIndex(selectedIndexRef.current, itemsRef.current.length));
    },
    [commitOnReleaseRef, itemsRef, openRef, openSwitcher, selectedIndexRef, setSelectedIndex],
  );

  const selectCurrentItem = useCallback(() => {
    selectItem(itemsRef.current[selectedIndexRef.current]);
  }, [itemsRef, selectItem, selectedIndexRef]);

  return {
    setOpen,
    setSelectedIndex,
    selectItem,
    openSwitcher,
    cycleSwitcher,
    cancelSwitcher,
    selectCurrentItem,
  };
}

function useRecentTaskSwitcherCommand(shortcut: KeyboardShortcut, openSwitcher: () => void) {
  const commands = useMemo(
    () => [
      {
        id: "open-recent-task-switcher",
        label: "Open Recent Task Switcher",
        group: "Navigation",
        shortcut,
        keywords: ["recent", "task", "switcher", "history"],
        action: openSwitcher,
      },
    ],
    [openSwitcher, shortcut],
  );
  useRegisterCommands(commands);
}

function useSwitcherGlobalKeyboard(refs: SwitcherRefs, actions: SwitcherActions) {
  const { cancelledRef, commitOnReleaseRef, openRef, shortcutRef } = refs;
  const { cancelSwitcher, cycleSwitcher, selectCurrentItem } = actions;

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && openRef.current) {
        event.preventDefault();
        event.stopPropagation();
        cancelSwitcher();
        return;
      }

      const currentShortcut = shortcutRef.current;
      if (!isCycleShortcutEvent(event, currentShortcut)) return;

      event.preventDefault();
      event.stopPropagation();
      cycleSwitcher(hasHoldModifier(currentShortcut));
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      const currentShortcut = shortcutRef.current;
      if (!openRef.current || !commitOnReleaseRef.current) return;
      if (!isCommitReleaseEvent(event, currentShortcut)) return;

      event.preventDefault();
      event.stopPropagation();
      if (cancelledRef.current) {
        cancelSwitcher();
        return;
      }
      selectCurrentItem();
    };

    const handleBlur = () => {
      if (openRef.current) cancelSwitcher();
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden" && openRef.current) cancelSwitcher();
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    window.addEventListener("blur", handleBlur);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      window.removeEventListener("blur", handleBlur);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [
    cancelSwitcher,
    cancelledRef,
    commitOnReleaseRef,
    cycleSwitcher,
    openRef,
    selectCurrentItem,
    shortcutRef,
  ]);
}

function useDialogKeyDown({
  actions,
  items,
  refs,
  selectedIndex,
}: {
  actions: SwitcherActions;
  items: RecentTaskDisplayItem[];
  refs: SwitcherRefs;
  selectedIndex: number;
}) {
  const { cancelSwitcher, selectItem, setSelectedIndex } = actions;
  const { selectedIndexRef } = refs;

  return useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setSelectedIndex(getNextSelectionIndex(selectedIndexRef.current, items.length));
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setSelectedIndex(getPreviousSelectionIndex(selectedIndexRef.current, items.length));
      }
      if (event.key === "Enter") {
        event.preventDefault();
        selectItem(items[selectedIndex]);
      }
      if (event.key === "Escape") {
        event.preventDefault();
        cancelSwitcher();
      }
    },
    [cancelSwitcher, items, selectItem, selectedIndex, selectedIndexRef, setSelectedIndex],
  );
}

function useRecentTaskSwitcherController(): RecentTaskSwitcherController {
  const router = useRouter();
  const [entries, setEntries] = useRecentTaskEntries();
  const [open, setOpenState] = useState(false);
  const [rawSelectedIndex, setRawSelectedIndex] = useState(-1);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();
  const keyboardShortcuts = useAppStore((state) => state.userSettings.keyboardShortcuts);
  const shortcut = getShortcut("TASK_SWITCHER", keyboardShortcuts);
  const context = useRecentTaskBuildContext();
  useRecordActiveTask(context, setEntries);

  const items = useMemo(() => buildRecentTaskDisplayItems(entries, context), [entries, context]);
  const selectedIndex = getResolvedSelectionIndex(rawSelectedIndex, items, context.activeTaskId);
  const refs = useSwitcherRefs(open, selectedIndex, items, context.activeTaskId, shortcut);
  const setSelectedIndex = useSelectedIndexSetter(refs, setRawSelectedIndex);
  const routeToTask = useCallback((taskId: string) => router.push(linkToTask(taskId)), [router]);
  const actions = useSwitcherActions({
    refs,
    routeToTask,
    setCommandPanelOpen,
    setOpenState,
    setSelectedIndex,
  });
  const { openSwitcher } = actions;
  const openFromCommand = useCallback(() => openSwitcher(false), [openSwitcher]);
  useRecentTaskSwitcherCommand(shortcut, openFromCommand);
  useSwitcherGlobalKeyboard(refs, actions);
  const handleKeyDown = useDialogKeyDown({ actions, items, refs, selectedIndex });

  return {
    open,
    setOpen: actions.setOpen,
    items,
    selectedIndex,
    setSelectedIndex,
    shortcutLabel: formatShortcut(shortcut),
    selectItem: actions.selectItem,
    handleKeyDown,
  };
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
