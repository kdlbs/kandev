import { DEMO_SCENARIO_VERSION } from "./scenario";

export function createDemoConversationRuntime(send: (socketId: string, message: unknown) => void) {
  const subscriptions = new Map<string, Map<string, string>>();
  const revisions = new Map<string, number>();
  const epoch = `browser-demo-${DEMO_SCENARIO_VERSION}`;

  return {
    route(socketId: string, action: string, payload: Record<string, unknown>) {
      const scopeId = String(payload.scope_id || "");
      if (action === "session.conversation.unsubscribe") {
        subscriptions.get(socketId)?.delete(scopeId);
        return { success: true };
      }
      if (action !== "session.conversation.subscribe") return null;
      const sessionId = String(payload.session_id || "");
      const scopes = subscriptions.get(socketId) ?? new Map<string, string>();
      scopes.set(scopeId, sessionId);
      subscriptions.set(socketId, scopes);
      return {
        success: true,
        protocol_version: 2,
        scope_id: scopeId,
        session_id: sessionId,
        epoch,
        revision: String(revisions.get(sessionId) ?? 0),
      };
    },
    publish(action: string, value: unknown) {
      if (action !== "session.message.added" && action !== "session.message.updated") return;
      const message = value as Record<string, unknown>;
      const sessionId = String(message.session_id);
      const base = revisions.get(sessionId) ?? 0;
      revisions.set(sessionId, base + 1);
      for (const [socketId, scopes] of subscriptions) {
        for (const [scopeId, subscribedSession] of scopes) {
          if (subscribedSession !== sessionId) continue;
          send(socketId, {
            type: "notification",
            action: "session.conversation.changed",
            payload: {
              protocol_version: 2,
              scope_id: scopeId,
              session_id: sessionId,
              epoch,
              base_revision: String(base),
              revision: String(base + 1),
              operations: [
                { kind: "upsert", entity: "message", id: String(message.message_id), message },
              ],
            },
          });
        }
      }
    },
    close(socketId: string) {
      subscriptions.delete(socketId);
    },
    reset() {
      subscriptions.clear();
      revisions.clear();
    },
  };
}
