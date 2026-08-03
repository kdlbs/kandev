import type { Page } from "@playwright/test";

type DroppedMessage = {
  action: string;
  content: string;
};

type PromptDropController = {
  dropPrompt: (prompt: string) => void;
  droppedCount: () => number;
};

type MessageAddResponseDropController = {
  dropNextMessageAddResponse: () => void;
  droppedCount: () => number;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : null;
}

function parseJSONFrames(message: string | Buffer): Array<Record<string, unknown>> {
  if (typeof message !== "string") return [];
  const frames: Array<Record<string, unknown>> = [];
  for (const part of message.split("\n")) {
    if (!part.trim()) continue;
    try {
      const parsed = asRecord(JSON.parse(part));
      if (parsed) frames.push(parsed);
    } catch {
      // Preserve non-JSON frames; the app's WS client handles parse failures too.
    }
  }
  return frames;
}

function isTargetUserMessageAdded(
  message: unknown,
  prompt: string,
): message is { payload: { content: string } } {
  const envelope = asRecord(message);
  const payload = asRecord(envelope?.payload);
  return (
    envelope?.action === "session.message.added" &&
    payload?.author_type === "user" &&
    typeof payload.content === "string" &&
    payload.content.includes(prompt)
  );
}

function filterServerFrame(
  message: string | Buffer,
  prompt: string | null,
  dropped: DroppedMessage[],
): string | Buffer | null {
  if (!prompt || typeof message !== "string") return message;

  const kept: string[] = [];
  let didDrop = false;
  for (const part of message.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) {
      kept.push(part);
      continue;
    }
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (isTargetUserMessageAdded(parsed, prompt)) {
        didDrop = true;
        dropped.push({ action: "session.message.added", content: parsed.payload.content });
        continue;
      }
    } catch {
      // Preserve non-JSON frames; the app's WS client handles parse failures too.
    }
    kept.push(part);
  }

  if (!didDrop) return message;
  const filtered = kept.join("\n");
  return filtered.trim() ? filtered : null;
}

export async function routeMainWebSocketWithPromptDrop(page: Page): Promise<PromptDropController> {
  let promptToDrop: string | null = null;
  const dropped: DroppedMessage[] = [];

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    server.onMessage((message) => {
      const filtered = filterServerFrame(message, promptToDrop, dropped);
      if (filtered !== null) ws.send(filtered);
    });
  });

  return {
    dropPrompt: (prompt: string) => {
      promptToDrop = prompt;
      dropped.length = 0;
    },
    droppedCount: () => dropped.length,
  };
}

function filterMessageAddResponses(
  message: string | Buffer,
  requestIDs: Set<string>,
  armed: { value: boolean },
  dropped: { value: number },
): string | Buffer | null {
  if (typeof message !== "string" || !armed.value) return message;

  const kept: string[] = [];
  let didDrop = false;
  for (const part of message.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) {
      kept.push(part);
      continue;
    }

    let responseID: string | undefined;
    try {
      const frame = asRecord(JSON.parse(trimmed));
      if (
        frame?.type === "response" &&
        frame.action === "message.add" &&
        typeof frame.id === "string" &&
        requestIDs.has(frame.id)
      ) {
        responseID = frame.id;
      }
    } catch {
      // Preserve non-JSON frames.
    }

    if (responseID) {
      requestIDs.delete(responseID);
      armed.value = false;
      dropped.value += 1;
      didDrop = true;
      continue;
    }
    kept.push(part);
  }

  if (!didDrop) return message;
  const filtered = kept.join("\n");
  return filtered.trim() ? filtered : null;
}

/**
 * Drops one correlated `message.add` response while preserving the request,
 * notification, and every unrelated frame. This exercises the UI's
 * stable-ID reconciliation path without disconnecting the whole socket.
 */
export async function routeMainWebSocketWithMessageAddResponseDrop(
  page: Page,
): Promise<MessageAddResponseDropController> {
  const requestIDs = new Set<string>();
  const armed = { value: false };
  const dropped = { value: 0 };

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      for (const frame of parseJSONFrames(message)) {
        if (
          armed.value &&
          frame.type === "request" &&
          frame.action === "message.add" &&
          typeof frame.id === "string"
        ) {
          requestIDs.add(frame.id);
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      const filtered = filterMessageAddResponses(message, requestIDs, armed, dropped);
      if (filtered !== null) ws.send(filtered);
    });
  });

  return {
    dropNextMessageAddResponse: () => {
      requestIDs.clear();
      armed.value = true;
      dropped.value = 0;
    },
    droppedCount: () => dropped.value,
  };
}
