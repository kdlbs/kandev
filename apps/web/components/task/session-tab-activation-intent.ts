const SESSION_TAB_USER_ACTIVATION_TTL_MS = 1500;
const NESTED_INTERACTIVE_SELECTOR =
  "button, a, input, select, textarea, [role='button'], [role='menuitem']";

type SessionTabActivationIntent = {
  expiresAt: number;
};

const sessionTabActivationIntents = new Map<string, SessionTabActivationIntent>();

export function shouldMarkSessionTabUserActivationIntent(args: {
  sessionId: string | null | undefined;
  isActive: boolean;
  target: EventTarget | null;
}): boolean {
  if (!args.sessionId || args.isActive) return false;
  if (typeof Element === "undefined" || !(args.target instanceof Element)) return true;
  return !args.target.closest(NESTED_INTERACTIVE_SELECTOR);
}

export function markSessionTabUserActivationIntent(sessionId: string | null | undefined): void {
  if (!sessionId) return;
  sessionTabActivationIntents.set(sessionId, {
    expiresAt: Date.now() + SESSION_TAB_USER_ACTIVATION_TTL_MS,
  });
}

export function consumeSessionTabUserActivationIntent(sessionId: string): boolean {
  const intent = sessionTabActivationIntents.get(sessionId);
  if (!intent) return false;
  const now = Date.now();
  if (intent.expiresAt < now) {
    sessionTabActivationIntents.delete(sessionId);
    return false;
  }
  sessionTabActivationIntents.delete(sessionId);
  return true;
}

export function clearSessionTabUserActivationIntentsForTest(): void {
  sessionTabActivationIntents.clear();
}
