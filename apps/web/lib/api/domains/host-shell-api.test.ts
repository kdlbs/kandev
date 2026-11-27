import { afterEach, describe, expect, it, vi } from "vitest";
import { startAgentLogin } from "./host-shell-api";

const originalFetch = global.fetch;
afterEach(() => {
  global.fetch = originalFetch;
});

describe("agent login region selection", () => {
  it.each([undefined, "cn", "global"])(
    "posts the registered variant %s with the terminal size",
    async (commandVariant) => {
      const fetchSpy = vi.fn(
        async () =>
          new Response(JSON.stringify({ session_id: "login", running: true }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      );
      global.fetch = fetchSpy as typeof global.fetch;
      const controller = new AbortController();
      const result = await startAgentLogin(
        "minimax-acp",
        { cols: 80, rows: 24 },
        {
          commandVariant,
          init: { signal: controller.signal },
        },
      );
      expect(fetchSpy).toHaveBeenCalledWith(
        "http://localhost:3000/api/v1/agent-login/agents/minimax-acp/start",
        expect.objectContaining({
          method: "POST",
          signal: controller.signal,
          body: JSON.stringify({ cols: 80, rows: 24, command_variant: commandVariant }),
        }),
      );
      expect(result.session_id).toBe("login");
    },
  );
});
