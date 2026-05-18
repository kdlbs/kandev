import { test, expect } from "../../fixtures/test-base";

/**
 * End-to-end coverage for the login-PTY flow:
 *
 *   POST /api/v1/agent-login/<name>/start  → spawns PTY, returns session_id
 *   WS  /api/v1/agent-login/<id>/stream    → bi-directional binary I/O
 *   POST /api/v1/agent-login/<id>/stop     → terminates and sends exit msg
 *
 * mock-agent's LoginCommand is `cat`, which echoes whatever we send back.
 * The PTY echo flag also enables terminal-style local echo, so a single
 * write of "ping\n" is reflected at least once in the readback.
 */
test.describe("agent login PTY", () => {
  test("start → echo input → stop", async ({ backend }) => {
    test.setTimeout(15_000);

    const startRes = await fetch(`${backend.baseUrl}/api/v1/agent-login/agents/mock-agent/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cols: 80, rows: 24 }),
    });
    expect(startRes.status).toBe(200);
    const session = (await startRes.json()) as {
      session_id: string;
      running: boolean;
      cmd: string[];
    };
    expect(session.session_id).toBeTruthy();
    expect(session.running).toBe(true);
    expect(session.cmd).toContain("cat");

    const wsUrl =
      backend.baseUrl.replace(/^http/, "ws") +
      `/api/v1/agent-login/sessions/${session.session_id}/stream`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";

    let received = "";
    let exitedCleanly = false;
    let exitCode: number | null = null;

    await new Promise<void>((resolve, reject) => {
      ws.addEventListener("open", () => resolve(), { once: true });
      ws.addEventListener("error", () => reject(new Error("ws error")), { once: true });
    });

    ws.addEventListener("message", (ev) => {
      if (typeof ev.data === "string") {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "exit") {
            exitedCleanly = true;
            exitCode = msg.exit_code ?? null;
          }
        } catch {
          // ignore non-JSON text
        }
        return;
      }
      received += new TextDecoder().decode(new Uint8Array(ev.data as ArrayBuffer));
    });

    // Send input.
    ws.send(new TextEncoder().encode("ping\n"));

    // Wait for echo. PTY in cooked mode echoes input back at least once.
    await waitFor(() => received.includes("ping"), 5_000, "echo");

    // Stop the session via HTTP — sends SIGTERM to cat, which exits 0.
    const stopRes = await fetch(
      `${backend.baseUrl}/api/v1/agent-login/sessions/${session.session_id}/stop`,
      { method: "POST" },
    );
    expect(stopRes.status).toBe(200);

    // Server should send a final "exit" text frame.
    await waitFor(() => exitedCleanly, 5_000, "exit message");
    ws.close();

    // exit_code may be 0 (clean exit on EOF) or non-zero (killed by signal).
    // Both are acceptable terminations — we just care that the server told us.
    expect(exitCode).not.toBeNull();
  });

  test("start for an unknown agent returns 404", async ({ backend }) => {
    test.setTimeout(5_000);
    const res = await fetch(`${backend.baseUrl}/api/v1/agent-login/agents/no-such-agent/start`, {
      method: "POST",
    });
    expect(res.status).toBe(404);
  });

  test("login start wraps the agent in a SIGINT-trapping shell (shell-drop wrapper)", async ({
    backend,
  }) => {
    test.setTimeout(5_000);

    // The wrapper is what lets Ctrl+C kill the agent without ending the
    // PTY session - sh traps SIGINT then execs $SHELL after the agent
    // exits. Whether the shell actually appears at the prompt is too
    // shell-specific to assert (zsh vs bash vs dash), but the wrapper
    // being in place is a structural property we *can* assert: it shows
    // up in the session's resolved argv.
    const startRes = await fetch(`${backend.baseUrl}/api/v1/agent-login/agents/mock-agent/start`, {
      method: "POST",
      body: JSON.stringify({ cols: 80, rows: 24 }),
    });
    const session = (await startRes.json()) as { session_id: string; cmd: string[] };

    // Expect the wrapper shape: sh -c '<script with trap INT and exec $SHELL>' <marker> <agent argv...>
    expect(session.cmd[0]).toBe("sh");
    expect(session.cmd[1]).toBe("-c");
    expect(session.cmd[2]).toContain("trap");
    expect(session.cmd[2]).toContain("INT");
    expect(session.cmd[2]).toContain("exec");
    expect(session.cmd).toContain("cat"); // mock-agent's actual login command

    await fetch(`${backend.baseUrl}/api/v1/agent-login/sessions/${session.session_id}/stop`, {
      method: "POST",
    });
  });

  test("mock-agent surfaces login_command on /agents/available", async ({ backend }) => {
    test.setTimeout(5_000);
    const res = await fetch(`${backend.baseUrl}/api/v1/agents/available`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as {
      agents: Array<{ name: string; login_command?: { cmd: string[]; description?: string } }>;
    };
    const mock = body.agents.find((a) => a.name === "mock-agent");
    expect(mock).toBeTruthy();
    expect(mock!.login_command).toBeTruthy();
    expect(mock!.login_command!.cmd).toContain("cat");
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
