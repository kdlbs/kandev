import type { Page } from "@playwright/test";
import { registerExpectedWsDrop } from "./ws-account";

type DroppedMessage = {
  action: string;
  content: string;
};

type PromptDropController = {
  dropPrompt: (prompt: string) => void;
  droppedCount: () => number;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : null;
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

function sessionIdFor(message: unknown): string | undefined {
  const payload = asRecord(asRecord(message)?.payload);
  return typeof payload?.session_id === "string" ? payload.session_id : undefined;
}

function filterServerFrame(
  message: string | Buffer,
  prompt: string | null,
  dropped: DroppedMessage[],
  onDrop: (message: unknown) => void,
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
        onDrop(parsed);
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
      const filtered = filterServerFrame(message, promptToDrop, dropped, (parsed) => {
        registerExpectedWsDrop(page, {
          type: "notification",
          action: "session.message.added",
          sessionId: sessionIdFor(parsed),
          reason: "intentional prompt WS-gap fault injection",
        });
      });
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
