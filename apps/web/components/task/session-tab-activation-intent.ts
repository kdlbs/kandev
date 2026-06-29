const SESSION_TAB_USER_ACTIVATION_TTL_MS = 1500;

type SessionTabActivationIntent = {
  expiresAt: number;
};

const sessionTabActivationIntents = new Map<string, SessionTabActivationIntent>();

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
