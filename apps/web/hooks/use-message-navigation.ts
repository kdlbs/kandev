import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type SetStateAction,
} from "react";
import type { RenderItem } from "@/hooks/use-processed-messages";

export type UserMessageNavigationOptions = {
  sessionId: string | null;
  items: RenderItem[];
  hasOlder: boolean;
  oldestCursor: string | null;
  loadOlder: () => Promise<number>;
  navigateTo: (messageId: string) => boolean | Promise<boolean>;
};

function getUserMessageIds(items: RenderItem[]): string[] {
  const ids: string[] = [];
  for (const item of items) {
    if (item.type === "message") {
      if (item.message.author_type === "user") ids.push(item.message.id);
      continue;
    }
    if (item.type === "turn_group") {
      for (const message of item.messages) {
        if (message.author_type === "user") ids.push(message.id);
      }
    }
  }
  return ids;
}

type NavigationSnapshot = {
  userMessageIds: string[];
  originId: string | null;
  hasOlder: boolean;
  oldestCursor: string | null;
};

function previousUserMessageId(snapshot: NavigationSnapshot): string | null {
  if (!snapshot.originId) return null;
  const index = snapshot.userMessageIds.indexOf(snapshot.originId);
  return index > 0 ? snapshot.userMessageIds[index - 1] : null;
}

type NavigationRuntime = {
  busyRef: RefObject<boolean>;
  generationRef: RefObject<number>;
  mountedRef: RefObject<boolean>;
  sessionIdRef: RefObject<string | null>;
  snapshotRef: RefObject<NavigationSnapshot>;
  loadOlderRef: RefObject<() => Promise<number>>;
  navigateToRef: RefObject<(messageId: string) => boolean | Promise<boolean>>;
  setIsBusy: Dispatch<SetStateAction<boolean>>;
  onDestination: (messageId: string) => void;
};

type ActiveAction = { generation: number; sessionId: string };

function beginAction(runtime: NavigationRuntime): ActiveAction | null {
  if (runtime.busyRef.current || !runtime.sessionIdRef.current) return null;
  runtime.busyRef.current = true;
  runtime.setIsBusy(true);
  return {
    generation: ++runtime.generationRef.current,
    sessionId: runtime.sessionIdRef.current,
  };
}

function isCurrentAction(runtime: NavigationRuntime, action: ActiveAction): boolean {
  return (
    runtime.mountedRef.current &&
    runtime.generationRef.current === action.generation &&
    runtime.sessionIdRef.current === action.sessionId
  );
}

function finishAction(runtime: NavigationRuntime, action: ActiveAction) {
  if (!isCurrentAction(runtime, action)) return;
  runtime.busyRef.current = false;
  runtime.setIsBusy(false);
}

async function focusDestination(
  runtime: NavigationRuntime,
  action: ActiveAction,
  destinationId: string,
) {
  const didNavigate = await runtime.navigateToRef.current(destinationId);
  if (!didNavigate || !isCurrentAction(runtime, action)) return;
  runtime.onDestination(destinationId);
}

const SNAPSHOT_COMMIT_ATTEMPTS = 120;
const SNAPSHOT_COMMIT_INTERVAL_MS = 16;

async function waitForSnapshotCommit(
  runtime: NavigationRuntime,
  action: ActiveAction,
  previousCursor: string | null,
) {
  for (let attempt = 0; attempt < SNAPSHOT_COMMIT_ATTEMPTS; attempt++) {
    if (!isCurrentAction(runtime, action)) return false;
    if (runtime.snapshotRef.current.oldestCursor !== previousCursor) return true;
    await new Promise<void>((resolve) => window.setTimeout(resolve, SNAPSHOT_COMMIT_INTERVAL_MS));
  }
  return runtime.snapshotRef.current.oldestCursor !== previousCursor;
}

async function runPreviousNavigation(runtime: NavigationRuntime) {
  const action = beginAction(runtime);
  if (!action) return;
  try {
    while (isCurrentAction(runtime, action)) {
      const snapshot = runtime.snapshotRef.current;
      const destinationId = previousUserMessageId(snapshot);
      if (destinationId) {
        await focusDestination(runtime, action, destinationId);
        return;
      }
      if (!snapshot.hasOlder) return;
      const previousCursor = snapshot.oldestCursor;
      const loaded = await runtime.loadOlderRef.current();
      if (!isCurrentAction(runtime, action) || loaded <= 0) return;
      if (!(await waitForSnapshotCommit(runtime, action, previousCursor))) return;
    }
  } catch {
    // Loading and scroll-adapter failures intentionally leave navigation retryable.
  } finally {
    finishAction(runtime, action);
  }
}

async function runNextNavigation(runtime: NavigationRuntime, destinationId: string | null) {
  if (!destinationId) return;
  const action = beginAction(runtime);
  if (!action) return;
  try {
    await focusDestination(runtime, action, destinationId);
  } catch {
    // A failed scroll adapter leaves the current viewport unchanged.
  } finally {
    finishAction(runtime, action);
  }
}

export function useUserMessageNavigation({
  items,
  hasOlder,
  oldestCursor,
  loadOlder,
  navigateTo,
  sessionId,
}: UserMessageNavigationOptions) {
  const userMessageIds = useMemo(() => getUserMessageIds(items), [items]);
  const [viewportOriginId, setViewportOriginState] = useState<string | null>(null);
  const [isBusy, setIsBusy] = useState(false);
  const busyRef = useRef(false);
  const actionGenerationRef = useRef(0);
  const mountedRef = useRef(true);
  const sessionIdRef = useRef(sessionId);
  const loadOlderRef = useRef(loadOlder);
  const navigateToRef = useRef(navigateTo);
  const setViewportOrigin = useCallback((messageId: string | null) => {
    setViewportOriginState(messageId);
  }, []);
  const originId =
    viewportOriginId && userMessageIds.includes(viewportOriginId)
      ? viewportOriginId
      : (userMessageIds[userMessageIds.length - 1] ?? null);
  const originIndex = originId ? userMessageIds.indexOf(originId) : -1;
  const previousId = originIndex > 0 ? userMessageIds[originIndex - 1] : null;
  const nextId =
    originIndex >= 0 && originIndex < userMessageIds.length - 1
      ? userMessageIds[originIndex + 1]
      : null;
  const snapshotRef = useRef<NavigationSnapshot>({
    userMessageIds,
    originId,
    hasOlder,
    oldestCursor,
  });
  snapshotRef.current = { userMessageIds, originId, hasOlder, oldestCursor };
  sessionIdRef.current = sessionId;
  loadOlderRef.current = loadOlder;
  navigateToRef.current = navigateTo;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      actionGenerationRef.current++;
    };
  }, []);

  useEffect(() => {
    actionGenerationRef.current++;
    busyRef.current = false;
    setIsBusy(false);
    setViewportOriginState(null);
  }, [sessionId]);

  const onDestination = useCallback((messageId: string) => {
    setViewportOriginState(messageId);
  }, []);
  const runtime: NavigationRuntime = {
    busyRef,
    generationRef: actionGenerationRef,
    mountedRef,
    sessionIdRef,
    snapshotRef,
    loadOlderRef,
    navigateToRef,
    setIsBusy,
    onDestination,
  };
  const runtimeRef = useRef(runtime);
  runtimeRef.current = runtime;
  const goPrevious = useCallback(() => runPreviousNavigation(runtimeRef.current), []);
  const goNext = useCallback(() => runNextNavigation(runtimeRef.current, nextId), [nextId]);
  return {
    userMessageIds,
    originId,
    setViewportOrigin,
    hasPrevious: originId !== null && (previousId !== null || hasOlder),
    previousId,
    hasNext: nextId !== null,
    nextId,
    isBusy,
    goPrevious,
    goNext,
  };
}
