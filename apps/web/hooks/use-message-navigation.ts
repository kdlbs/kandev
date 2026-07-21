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
    }
  }
  return ids;
}

type NavigationSnapshot = {
  userMessageIds: string[];
  hasOlder: boolean;
  oldestCursor: string | null;
};

function previousUserMessageId(snapshot: NavigationSnapshot, originId: string): string | null {
  const index = snapshot.userMessageIds.indexOf(originId);
  return index > 0 ? snapshot.userMessageIds[index - 1] : null;
}

function nextUserMessageId(snapshot: NavigationSnapshot, originId: string): string | null {
  const index = snapshot.userMessageIds.indexOf(originId);
  return index >= 0 && index < snapshot.userMessageIds.length - 1
    ? snapshot.userMessageIds[index + 1]
    : null;
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

async function runPreviousNavigation(runtime: NavigationRuntime, originId: string) {
  if (!runtime.snapshotRef.current.userMessageIds.includes(originId)) return;
  const action = beginAction(runtime);
  if (!action) return;
  try {
    while (isCurrentAction(runtime, action)) {
      const snapshot = runtime.snapshotRef.current;
      const destinationId = previousUserMessageId(snapshot, originId);
      if (destinationId) {
        await runtime.navigateToRef.current(destinationId);
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

async function runNextNavigation(runtime: NavigationRuntime, originId: string) {
  const destinationId = nextUserMessageId(runtime.snapshotRef.current, originId);
  if (!destinationId) return;
  const action = beginAction(runtime);
  if (!action) return;
  try {
    await runtime.navigateToRef.current(destinationId);
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
  const [isBusy, setIsBusy] = useState(false);
  const busyRef = useRef(false);
  const actionGenerationRef = useRef(0);
  const mountedRef = useRef(true);
  const sessionIdRef = useRef(sessionId);
  const loadOlderRef = useRef(loadOlder);
  const navigateToRef = useRef(navigateTo);
  const snapshotRef = useRef<NavigationSnapshot>({
    userMessageIds,
    hasOlder,
    oldestCursor,
  });
  snapshotRef.current = { userMessageIds, hasOlder, oldestCursor };
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
  }, [sessionId]);

  const runtime: NavigationRuntime = {
    busyRef,
    generationRef: actionGenerationRef,
    mountedRef,
    sessionIdRef,
    snapshotRef,
    loadOlderRef,
    navigateToRef,
    setIsBusy,
  };
  const runtimeRef = useRef(runtime);
  runtimeRef.current = runtime;
  const canNavigatePrevious = useCallback(
    (messageId: string) => {
      const index = userMessageIds.indexOf(messageId);
      return index >= 0 && (index > 0 || hasOlder);
    },
    [hasOlder, userMessageIds],
  );
  const canNavigateNext = useCallback(
    (messageId: string) => {
      const index = userMessageIds.indexOf(messageId);
      return index >= 0 && index < userMessageIds.length - 1;
    },
    [userMessageIds],
  );
  const goPrevious = useCallback(
    (messageId: string) => runPreviousNavigation(runtimeRef.current, messageId),
    [],
  );
  const goNext = useCallback(
    (messageId: string) => runNextNavigation(runtimeRef.current, messageId),
    [],
  );
  return {
    userMessageIds,
    canNavigatePrevious,
    canNavigateNext,
    isBusy,
    goPrevious,
    goNext,
  };
}
