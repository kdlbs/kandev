import type { Page } from "@playwright/test";

type MessageEnvelope = {
  action?: unknown;
  id?: unknown;
  payload?: unknown;
  type?: unknown;
};

function asEnvelope(value: unknown): MessageEnvelope | null {
  return typeof value === "object" && value !== null ? (value as MessageEnvelope) : null;
}

function parseFrame(message: string | Buffer): MessageEnvelope[] | null {
  if (typeof message !== "string") return null;
  const frames: MessageEnvelope[] = [];
  for (const part of message.split("\n")) {
    if (!part.trim()) continue;
    try {
      const parsed = asEnvelope(JSON.parse(part));
      if (parsed) frames.push(parsed);
    } catch {
      return null;
    }
  }
  return frames;
}

function rewriteStatusResponses(
  message: string | Buffer,
  requestIDs: Set<string>,
  embeddedVscodeSupported: boolean,
): string | Buffer {
  if (typeof message !== "string") return message;

  let changed = false;
  const rewritten = message
    .split("\n")
    .map((part) => {
      if (!part.trim()) return part;
      try {
        const envelope = asEnvelope(JSON.parse(part));
        if (
          envelope?.type !== "response" ||
          typeof envelope.id !== "string" ||
          !requestIDs.delete(envelope.id) ||
          !asEnvelope(envelope.payload)
        ) {
          return part;
        }
        changed = true;
        return JSON.stringify({
          ...envelope,
          payload: {
            ...envelope.payload,
            capabilities: { embedded_vscode: embeddedVscodeSupported },
          },
        });
      } catch {
        return part;
      }
    })
    .join("\n");

  return changed ? rewritten : message;
}

/**
 * Rewrites only task.session.status responses, preserving the real WebSocket
 * connection and every unrelated frame. This lets browser tests exercise the
 * session capability contract without duplicating executor policy in E2E code.
 */
export async function routeSessionEmbeddedVscodeCapability(
  page: Page,
  embeddedVscodeSupported: boolean,
): Promise<void> {
  const statusRequestIDs = new Set<string>();
  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      for (const envelope of parseFrame(message) ?? []) {
        if (
          envelope.type === "request" &&
          envelope.action === "task.session.status" &&
          typeof envelope.id === "string"
        ) {
          statusRequestIDs.add(envelope.id);
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      ws.send(rewriteStatusResponses(message, statusRequestIDs, embeddedVscodeSupported));
    });
  });
}
