import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

// The mock agent mirrors Claude Code: after each turn it forwards this
// prompt_suggestion through the _claude/sdkMessage extension notification.
export const NATIVE_SUGGESTION = "Yes, run the tests";

export async function setPromptSuggestions(apiClient: ApiClient, enabled: boolean) {
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    prompt_suggestions: enabled,
  });
  expect(response.ok).toBe(true);
}

/**
 * Collects gateway request actions the page sends. Call before the first
 * navigation: Playwright only reports sockets opened after registration.
 * Inspect the returned array at the interval boundaries of a negative
 * assertion, for example after a fixture's legitimate prompt has completed.
 */
export function observeGatewayRequests(page: Page): string[] {
  const actions: string[] = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", (event) => {
      const text =
        typeof event.payload === "string" ? event.payload : event.payload.toString("utf8");
      try {
        const frame = JSON.parse(text) as { type?: string; action?: string };
        if (frame.type === "request" && frame.action) actions.push(frame.action);
      } catch {
        // Terminal and other non-JSON frames are not gateway requests.
      }
    });
  });
  return actions;
}
