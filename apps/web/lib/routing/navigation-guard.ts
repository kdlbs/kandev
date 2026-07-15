export type NavigationIntent = {
  proceed: () => void;
  cancel: () => void;
};

export type NavigationBlocker = (intent: NavigationIntent) => void;

const HISTORY_POSITION_KEY = "__kandevNavigationPosition";

let activeBlocker: NavigationBlocker | null = null;
let currentPosition: number | null = null;
let popstateInstalled = false;
let historyMutationsTracked = false;
let allowedPop = false;
let restoringPop = false;

type BlockedPop = {
  delta: number;
  restorePosition: number;
  restored: boolean;
  canceled: boolean;
  proceedRequested: boolean;
};

let blockedPop: BlockedPop | null = null;

export function setNavigationBlocker(blocker: NavigationBlocker | null): () => void {
  ensureHistoryTracking();
  activeBlocker = blocker;
  return () => {
    if (activeBlocker === blocker) activeBlocker = null;
  };
}

export function requestNavigation(proceed: () => void): void {
  if (!activeBlocker) {
    proceed();
    return;
  }

  let settled = false;
  activeBlocker({
    proceed: () => {
      if (settled) return;
      settled = true;
      proceed();
    },
    cancel: () => {
      settled = true;
    },
  });
}

export function runWithNavigationBlockerBypassed(proceed: () => void): void {
  const blocker = activeBlocker;
  activeBlocker = null;
  try {
    proceed();
  } finally {
    activeBlocker = blocker;
  }
}

export function pushNavigationState(
  state: unknown,
  title: string,
  url?: string | URL | null,
  onNavigated?: () => void,
): void {
  ensureHistoryTracking();
  requestNavigation(() => {
    const nextPosition = (currentPosition ?? 0) + 1;
    window.history.pushState(withPosition(state, nextPosition), title, url);
    currentPosition = nextPosition;
    onNavigated?.();
  });
}

export function replaceNavigationState(
  state: unknown,
  title: string,
  url?: string | URL | null,
  onNavigated?: () => void,
): void {
  ensureHistoryTracking();
  requestNavigation(() => {
    const position = currentPosition ?? 0;
    window.history.replaceState(withPosition(state, position), title, url);
    onNavigated?.();
  });
}

export function clearNavigationBlockerForTests(): void {
  activeBlocker = null;
  currentPosition = null;
  allowedPop = false;
  restoringPop = false;
  blockedPop = null;
}

function ensureHistoryTracking(): void {
  if (typeof window === "undefined") return;

  trackNativeHistoryMutations();

  const statePosition = readPosition(window.history.state);
  if (currentPosition === null) {
    currentPosition = statePosition ?? 0;
    if (statePosition === null) {
      window.history.replaceState(withPosition(window.history.state, currentPosition), "");
    }
  }

  if (!popstateInstalled) {
    window.addEventListener("popstate", handlePopState, true);
    popstateInstalled = true;
  }
}

function handlePopState(event: PopStateEvent): void {
  const targetPosition = readPosition(event.state);
  if (targetPosition === null) {
    if (activeBlocker && currentPosition !== null) {
      // Entries created before tracking was installed are behind the current,
      // tagged settings entry. Restore the current entry before prompting.
      blockPop(event, -1, currentPosition);
    }
    return;
  }

  if (allowedPop) {
    allowedPop = false;
    currentPosition = targetPosition;
    return;
  }

  if (restoringPop) {
    if (targetPosition !== blockedPop?.restorePosition) {
      restoringPop = false;
    } else {
      restoringPop = false;
      currentPosition = targetPosition;
      finishPopRestoration();
      return;
    }
  }

  const fromPosition = currentPosition ?? targetPosition;
  const delta = targetPosition - fromPosition;
  if (!activeBlocker || delta === 0) {
    currentPosition = targetPosition;
    return;
  }

  blockPop(event, delta, fromPosition);
}

function blockPop(event: PopStateEvent, delta: number, restorePosition: number): void {
  const blocker = activeBlocker;
  if (!blocker) return;
  event.stopImmediatePropagation();
  const pending: BlockedPop = {
    delta,
    restorePosition,
    restored: false,
    canceled: false,
    proceedRequested: false,
  };
  blockedPop = pending;
  restoringPop = true;
  window.history.go(-delta);

  blocker({
    proceed: () => proceedBlockedPop(pending),
    cancel: () => cancelBlockedPop(pending),
  });
}

function trackNativeHistoryMutations(): void {
  if (historyMutationsTracked) return;
  historyMutationsTracked = true;
  const pushState = window.history.pushState.bind(window.history);
  const replaceState = window.history.replaceState.bind(window.history);
  window.history.pushState = (state, title, url) => {
    const position = readPosition(state) ?? (currentPosition ?? 0) + 1;
    pushState(withPosition(state, position), title, url);
    currentPosition = position;
  };
  window.history.replaceState = (state, title, url) => {
    const position = readPosition(state) ?? currentPosition ?? 0;
    replaceState(withPosition(state, position), title, url);
    currentPosition = position;
  };
}

function proceedBlockedPop(pending: BlockedPop): void {
  if (pending.canceled || blockedPop !== pending) return;
  if (!pending.restored) {
    pending.proceedRequested = true;
    return;
  }
  blockedPop = null;
  allowedPop = true;
  window.history.go(pending.delta);
}

function cancelBlockedPop(pending: BlockedPop): void {
  if (blockedPop !== pending) return;
  pending.canceled = true;
  if (pending.restored) blockedPop = null;
}

function finishPopRestoration(): void {
  const pending = blockedPop;
  if (!pending) return;
  pending.restored = true;
  if (pending.canceled) {
    blockedPop = null;
  } else if (pending.proceedRequested) {
    proceedBlockedPop(pending);
  }
}

function readPosition(state: unknown): number | null {
  if (!state || typeof state !== "object") return null;
  const position = (state as Record<string, unknown>)[HISTORY_POSITION_KEY];
  return typeof position === "number" ? position : null;
}

function withPosition(state: unknown, position: number): Record<string, unknown> {
  const source = state && typeof state === "object" ? state : {};
  return { ...source, [HISTORY_POSITION_KEY]: position };
}

// client-router imports this module before subscribing to popstate, so eager
// installation lets the guard stop blocked traversals before route subscribers run.
if (typeof window !== "undefined") ensureHistoryTracking();
