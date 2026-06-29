const SESSION_TAB_USER_ACTIVATION_TTL_MS = 1500;

type SessionTabActivationIntent = {
  sessionId: string;
  expiresAt: number;
};

let sessionTabActivationIntent: SessionTabActivationIntent | null = null;

export function markSessionTabUserActivationIntent(sessionId: string | null | undefined): void {
  if (!sessionId) return;
  sessionTabActivationIntent = {
    sessionId,
    expiresAt: Date.now() + SESSION_TAB_USER_ACTIVATION_TTL_MS,
  };
}

export function consumeSessionTabUserActivationIntent(sessionId: string): boolean {
  const intent = sessionTabActivationIntent;
  if (!intent) return false;
  const now = Date.now();
  if (intent.expiresAt < now) {
    sessionTabActivationIntent = null;
    return false;
  }
  if (intent.sessionId !== sessionId) return false;
  sessionTabActivationIntent = null;
  return true;
}
