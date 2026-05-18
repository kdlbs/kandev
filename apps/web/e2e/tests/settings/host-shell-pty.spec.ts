import { test, expect } from "../../fixtures/test-base";

/**
 * End-to-end coverage for the host shell PTY - the dialog opened by the
 * "Terminal" button on the Agents settings page (and by the auth-recovery
 * banner when an agent has no registered LoginCommand).
 *
 *   POST /api/v1/host-shell/start          spawns $SHELL under a PTY
 *   WS  /api/v1/agent-login/sessions/:id/stream  same routes as the
 *                                           login PTY (shared manager)
 *   POST /api/v1/agent-login/sessions/:id/stop   terminates the session
 */
test.describe("host shell PTY", () => {
  test("start → run a command → stop", async ({ backend }) => {
    test.setTimeout(15_000);

    const startRes = await fetch(`${backend.baseUrl}/api/v1/host-shell/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cols: 80, rows: 24 }),
    });
    expect(startRes.status).toBe(200);
    const session = (await startRes.json()) as {
      session_id: string;
      agent_id: string;
      cmd: string[];
      running: boolean;
    };
    expect(session.session_id).toBeTruthy();
    expect(session.running).toBe(true);
    // The agent_id is the synthetic key the manager uses for shell sessions.
    expect(session.agent_id).toBe("_host_shell");
    // Cmd should be some shell binary - exact path varies by host.
    expect(session.cmd.length).toBeGreaterThan(0);

    const wsUrl =
      backend.baseUrl.replace(/^http/, "ws") +
      `/api/v1/agent-login/sessions/${session.session_id}/stream`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";

    let received = "";
    let exitedCleanly = false;
    ws.addEventListener("message", (ev) => {
      if (typeof ev.data === "string") {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "exit") exitedCleanly = true;
        } catch {
          // ignore
        }
        return;
      }
      received += new TextDecoder().decode(new Uint8Array(ev.data as ArrayBuffer));
    });
    await new Promise<void>((resolve, reject) => {
      ws.addEventListener("open", () => resolve(), { once: true });
      ws.addEventListener("error", () => reject(new Error("ws error")), { once: true });
    });

    // Give the shell a moment to print its prompt, then run a marker.
    await new Promise((r) => setTimeout(r, 300));
    ws.send(new TextEncoder().encode("echo HELLO_FROM_HOST_SHELL\n"));
    await waitFor(() => received.includes("HELLO_FROM_HOST_SHELL"), 8_000, "shell echo");

    const stopRes = await fetch(
      `${backend.baseUrl}/api/v1/agent-login/sessions/${session.session_id}/stop`,
      { method: "POST" },
    );
    expect(stopRes.status).toBe(200);
    await waitFor(() => exitedCleanly, 5_000, "exit message");
    ws.close();
  });

  test("a second start while one is running returns the same session (idempotent)", async ({
    backend,
  }) => {
    test.setTimeout(10_000);

    const first = (await (
      await fetch(`${backend.baseUrl}/api/v1/host-shell/start`, {
        method: "POST",
        body: JSON.stringify({ cols: 80, rows: 24 }),
      })
    ).json()) as { session_id: string };

    const second = (await (
      await fetch(`${backend.baseUrl}/api/v1/host-shell/start`, {
        method: "POST",
        body: JSON.stringify({ cols: 80, rows: 24 }),
      })
    ).json()) as { session_id: string };

    expect(second.session_id).toBe(first.session_id);

    await fetch(`${backend.baseUrl}/api/v1/agent-login/sessions/${first.session_id}/stop`, {
      method: "POST",
    });
  });
});

async function waitFor(check: () => boolean, timeoutMs: number, label: string) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 50));
  }
  throw new Error(`Timed out waiting for ${label}`);
}
