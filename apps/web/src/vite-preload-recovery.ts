const PRELOAD_ERROR_EVENT = "vite:preloadError";
const PRELOAD_RECOVERY_KEY = "kandev.preloadRecovery";
const PRELOAD_RECOVERY_TTL_MS = 60_000;

type RecoveryStorage = Pick<Storage, "getItem" | "setItem">;
type RecoveryEventTarget = Pick<EventTarget, "addEventListener" | "removeEventListener">;

interface RecoveryMarker {
  attemptedAt: number;
  href: string;
}

interface VitePreloadRecoveryOptions {
  target?: RecoveryEventTarget;
  getStorage?: () => RecoveryStorage;
  now?: () => number;
  getHref?: () => string;
  reload?: () => void;
}

function parseRecoveryMarker(value: string | null): RecoveryMarker | null {
  if (value === null) {
    return null;
  }

  try {
    const marker: unknown = JSON.parse(value);
    if (
      typeof marker === "object" &&
      marker !== null &&
      typeof (marker as RecoveryMarker).attemptedAt === "number" &&
      Number.isFinite((marker as RecoveryMarker).attemptedAt) &&
      typeof (marker as RecoveryMarker).href === "string"
    ) {
      return marker as RecoveryMarker;
    }
  } catch {
    // A corrupt marker is equivalent to no prior recovery attempt.
  }

  return null;
}

function isRecent(marker: RecoveryMarker | null, now: number): boolean {
  return (
    marker !== null &&
    marker.attemptedAt <= now &&
    now - marker.attemptedAt < PRELOAD_RECOVERY_TTL_MS
  );
}

export function installVitePreloadRecovery({
  target = window,
  getStorage = () => window.sessionStorage,
  now = Date.now,
  getHref = () => window.location.href,
  reload = () => window.location.reload(),
}: VitePreloadRecoveryOptions = {}): () => void {
  const handlePreloadError: EventListener = (event) => {
    let storage: RecoveryStorage;
    let storedMarker: string | null;

    try {
      storage = getStorage();
      storedMarker = storage.getItem(PRELOAD_RECOVERY_KEY);
    } catch {
      return;
    }

    const attemptedAt = now();
    if (isRecent(parseRecoveryMarker(storedMarker), attemptedAt)) {
      return;
    }

    const marker = JSON.stringify({ attemptedAt, href: getHref() } satisfies RecoveryMarker);
    try {
      storage.setItem(PRELOAD_RECOVERY_KEY, marker);
      if (storage.getItem(PRELOAD_RECOVERY_KEY) !== marker) {
        return;
      }
    } catch {
      return;
    }

    event.preventDefault();
    reload();
  };

  target.addEventListener(PRELOAD_ERROR_EVENT, handlePreloadError);
  return () => target.removeEventListener(PRELOAD_ERROR_EVENT, handlePreloadError);
}
