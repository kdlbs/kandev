import { getWebSocketClient } from "@/lib/ws/connection";

const CANCEL_TIMEOUT_MS = 15_000;

/**
 * The session-cancel action the chat panel's Stop uses (`agent.cancel`),
 * rejecting on any failure so a caller can show it. Not fenced against a
 * stale turn: a later turn on the session is cancelled the same way.
 */
export async function stopSessionTurn(sessionId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error("no websocket client");
  await client.request("agent.cancel", { session_id: sessionId }, CANCEL_TIMEOUT_MS);
}
