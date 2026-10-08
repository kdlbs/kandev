export type NavigationIntent = {
  proceed: () => void;
  cancel: () => void;
};

export type NavigationBlocker = (intent: NavigationIntent) => void;

const HISTORY_POSITION_KEY = "__kandevNavigationPosition";
const SHALLOW_ROUTE_KEY = "__kandevShallowRoute";

let activeBlocker: NavigationBlocker | null = null;
let currentPosition: number | null = null;
let popstateInstalled = false;
let historyMutationsTracked = false;
let allowedPop = false;
let restoringPop = false;
let currentShallowRoute: string | null = null;
// Position of a blocked-pop restoration whose traversal is still in flight
// when the intent was cancelled. The delayed popstate for that position must
// be consumed without touching currentPosition, so a newer navigation (e.g. a
// push performed right after cancel) is not clobbered.
let canceledRestorationPosition: number | null = null;

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

/** Marks the current history entry as part of a route-local navigation stack. */
export function markCurrentNavigationRoute(route: string): void {
  if (typeof window === "undefined") return;
  ensureHistoryTracking();
  window.history.replaceState(withShallowRoute(window.history.state, route), "");
}

type NavigationStateOptions = {
  bypassBlocker?: boolean;
  shallowRoute?: string;
};

export function pushNavigationState(
  state: unknown,
  title: string,
  url?: string | URL | null,
  onNavigated?: () => void,
  options?: NavigationStateOptions,
): void {
  ensureHistoryTracking();
  const push = () => {
    const nextPosition = (currentPosition ?? 0) + 1;
    const nextState = withShallowRoute(state, options?.shallowRoute);
    window.history.pushState(withPosition(nextState, nextPosition), title, url);
    currentPosition = nextPosition;
    onNavigated?.();
  };
  if (options?.bypassBlocker) push();
  else requestNavigation(push);
}

export function replaceNavigationState(
  state: unknown,
  title: string,
  url?: string | URL | null,
  onNavigated?: () => void,
  options?: NavigationStateOptions,
): void {
  ensureHistoryTracking();
  const replace = () => {
    const position = currentPosition ?? 0;
    const nextState = withShallowRoute(state, options?.shallowRoute);
    window.history.replaceState(withPosition(nextState, position), title, url);
    onNavigated?.();
  };
  if (options?.bypassBlocker) replace();
  else requestNavigation(replace);
}

export function clearNavigationBlockerForTests(): void {
  activeBlocker = null;
  currentPosition = null;
  allowedPop = false;
  restoringPop = false;
  currentShallowRoute = null;
  blockedPop = null;
  canceledRestorationPosition = null;
}

function ensureHistoryTracking(): void {
  if (typeof window === "undefined") return;

  trackNativeHistoryMutations();

  const statePosition = readPosition(window.history.state);
  if (currentPosition === null) {
    currentPosition = statePosition ?? 0;
    currentShallowRoute = readShallowRoute(window.history.state);
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
    if (allowedPop) {
      allowedPop = false;
      return;
    }
    if (activeBlocker && currentPosition !== null) {
      // Entries created before tracking was installed are behind the current,
      // tagged settings entry. Restore the current entry before prompting.
      blockPop(event, -1, currentPosition);
    }
    return;
  }

  if (targetPosition === canceledRestorationPosition) {
    // Popstate for a blocked-pop restoration whose intent was cancelled: a
    // newer navigation may have happened since, so consume it without
    // updating currentPosition.
    canceledRestorationPosition = null;
    return;
  }

  if (allowedPop) {
    allowedPop = false;
    currentPosition = targetPosition;
    currentShallowRoute = readShallowRoute(event.state);
    return;
  }

  if (restoringPop) {
    if (targetPosition !== blockedPop?.restorePosition) {
      restoringPop = false;
    } else {
      restoringPop = false;
      currentPosition = targetPosition;
      currentShallowRoute = readShallowRoute(event.state);
      finishPopRestoration();
      return;
    }
  }

  const fromPosition = currentPosition ?? targetPosition;
  const delta = targetPosition - fromPosition;
  const targetShallowRoute = readShallowRoute(event.state);
  if (targetShallowRoute && targetShallowRoute === currentShallowRoute) {
    currentPosition = targetPosition;
    return;
  }
  if (!activeBlocker || delta === 0) {
    currentPosition = targetPosition;
    currentShallowRoute = targetShallowRoute;
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
  canceledRestorationPosition = null; // a new block supersedes any stale one
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
    currentShallowRoute = readShallowRoute(state);
    // A new navigation supersedes any blocked-pop traversal still in flight;
    // without this the cancelled restoration position could eat a later
    // legitimate popstate to the same position.
    canceledRestorationPosition = null;
  };
  window.history.replaceState = (state, title, url) => {
    const position = readPosition(state) ?? currentPosition ?? 0;
    replaceState(withPosition(state, position), title, url);
    currentPosition = position;
    currentShallowRoute = readShallowRoute(state);
    canceledRestorationPosition = null;
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
  if (!pending.restored) {
    // The history.go(-delta) restoration is still in flight. Its popstate
    // must not overwrite a newer currentPosition (e.g. a push performed
    // right after cancel), so consume that exact position when it arrives.
    canceledRestorationPosition = pending.restorePosition;
    restoringPop = false;
  }
  blockedPop = null;
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

function readShallowRoute(state: unknown): string | null {
  if (!state || typeof state !== "object") return null;
  const route = (state as Record<string, unknown>)[SHALLOW_ROUTE_KEY];
  return typeof route === "string" ? route : null;
}

function withShallowRoute(state: unknown, route: string | undefined): Record<string, unknown> {
  const source = state && typeof state === "object" ? state : {};
  const next = { ...source } as Record<string, unknown>;
  if (route) next[SHALLOW_ROUTE_KEY] = route;
  else delete next[SHALLOW_ROUTE_KEY];
  return next;
}

function withPosition(state: unknown, position: number): Record<string, unknown> {
  const source = state && typeof state === "object" ? state : {};
  return { ...source, [HISTORY_POSITION_KEY]: position };
}

// client-router imports this module before subscribing to popstate, so eager
// installation lets the guard stop blocked traversals before route subscribers run.
if (typeof window !== "undefined") ensureHistoryTracking();
