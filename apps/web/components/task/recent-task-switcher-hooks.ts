"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type MutableRefObject,
} from "react";
import { useRouter } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import { useUserSettings } from "@/hooks/domains/settings/use-user-settings";
import { useAllRepositories } from "@/hooks/domains/workspace/use-all-repositories";
import {
  useActiveWorkflowSteps,
  useAllKanbanTasks,
  useKanbanSnapshots,
} from "@/hooks/domains/kanban/use-kanban-tasks";
import { useWorkflowItems } from "@/hooks/domains/kanban/use-kanban-snapshots";
import { useAllTaskSessionsByTaskFromCache } from "@/hooks/domains/session/use-task-session-by-id";
import { useStablePrimarySessionIds } from "@/hooks/domains/session/use-messages-by-session-cache";
import { useGitStatusByEnvFromCache } from "@/hooks/domains/session/use-git-status-cache";
import { useCommandPanelOpen } from "@/lib/commands/command-registry";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import { linkToTask } from "@/lib/links";
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

export type RecentTaskSwitcherController = {
  open: boolean;
  setOpen: (open: boolean) => void;
  items: RecentTaskDisplayItem[];
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
  shortcutLabel: string;
  selectItem: (item: RecentTaskDisplayItem | undefined) => void;
  handleKeyDown: (event: ReactKeyboardEvent) => void;
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

  return entries;
}

function useRecentTaskBuildContext(): RecentTaskBuildContext {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  // The active workflow selection is client-only; its steps come from the TQ
  // snapshot cache via useActiveWorkflowSteps.
  const kanbanWorkflowId = useAppStore((state) => state.workflows.activeId);
  const kanbanTasks = useAllKanbanTasks();
  const kanbanSteps = useActiveWorkflowSteps();
  const snapshots = useKanbanSnapshots();
  const workflows = useWorkflowItems(activeWorkspaceId);
  const { byWorkspaceId: repositoriesByWorkspace } = useAllRepositories(false);
  const sessionsByTaskId = useAllTaskSessionsByTaskFromCache();
  const environmentIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  // Git indicators read the TQ git cache keyed by environment (bridge-populated),
  // resolved per primary session of the known kanban tasks. Observe-only.
  const recentPrimarySessionIds = useStablePrimarySessionIds(kanbanTasks);
  const gitStatusByEnvId = useGitStatusByEnvFromCache(recentPrimarySessionIds);

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

function useRecordActiveTask(context: RecentTaskBuildContext) {
  const lastTaskIdRef = useRef<string | null>(null);
  const lastSignatureRef = useRef<string | null>(null);
  const lastEntryRef = useRef<RecentTaskEntry | undefined>(undefined);

  useEffect(() => {
    if (!context.activeTaskId) return;

    const isNewVisit = lastTaskIdRef.current !== context.activeTaskId;
    const previous = isNewVisit
      ? getRecentTasks().find((entry) => entry.taskId === context.activeTaskId)
      : lastEntryRef.current;
    const visitedAt = isNewVisit ? new Date().toISOString() : undefined;
    const entry = buildRecentTaskEntry(context.activeTaskId, context, previous, visitedAt);
    const signature = getEntrySignature(entry);

    if (!isNewVisit && lastSignatureRef.current === signature) return;

    lastTaskIdRef.current = context.activeTaskId;
    lastSignatureRef.current = signature;
    lastEntryRef.current = entry;
    upsertRecentTask(entry);
  }, [context]);
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
    (event: ReactKeyboardEvent) => {
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

export function useRecentTaskSwitcherController(): RecentTaskSwitcherController {
  const router = useRouter();
  const entries = useRecentTaskEntries();
  const [open, setOpenState] = useState(false);
  const [rawSelectedIndex, setRawSelectedIndex] = useState(-1);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();
  const keyboardShortcuts = useUserSettings().data?.keyboardShortcuts ?? {};
  const shortcut = getShortcut("TASK_SWITCHER", keyboardShortcuts);
  const context = useRecentTaskBuildContext();
  useRecordActiveTask(context);

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
