import { test, expect } from "../../fixtures/test-base";

type HostShellSession = {
  session_id: string;
  agent_id: string;
  cmd: string[];
  running: boolean;
};

type HostShellStream = {
  ws: WebSocket;
  getOutput: () => string;
  hasExited: () => boolean;
};

async function startHostShell(baseUrl: string, clientId?: string): Promise<HostShellSession> {
  const response = await fetch(`${baseUrl}/api/v1/host-shell/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ cols: 80, rows: 24, ...(clientId ? { client_id: clientId } : {}) }),
  });
  expect(response.status).toBe(200);
  return (await response.json()) as HostShellSession;
}

async function stopHostShell(baseUrl: string, sessionId: string): Promise<void> {
  await fetch(`${baseUrl}/api/v1/agent-login/sessions/${sessionId}/stop`, {
    method: "POST",
  });
}

async function openHostShellStream(baseUrl: string, sessionId: string): Promise<HostShellStream> {
  const wsUrl = baseUrl.replace(/^http/, "ws") + `/api/v1/agent-login/sessions/${sessionId}/stream`;
  const ws = new WebSocket(wsUrl);
  ws.binaryType = "arraybuffer";
  let output = "";
  let exited = false;
  ws.addEventListener("message", (event) => {
    if (typeof event.data === "string") {
      try {
        if ((JSON.parse(event.data) as { type?: string }).type === "exit") exited = true;
      } catch {
        // Ignore non-JSON control frames.
      }
      return;
    }
    output += new TextDecoder().decode(new Uint8Array(event.data as ArrayBuffer));
  });
  await new Promise<void>((resolve, reject) => {
    ws.addEventListener("open", () => resolve(), { once: true });
    ws.addEventListener("error", () => reject(new Error("host shell websocket error")), {
      once: true,
    });
  });
  return { ws, getOutput: () => output, hasExited: () => exited };
}

async function waitFor(check: () => boolean, timeoutMs: number, label: string) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (check()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error(`Timed out waiting for ${label}`);
}

test.describe("host shell PTY", () => {
  test("starts, streams a command, and stops with the legacy singleton identity", async ({
    backend,
  }) => {
    test.setTimeout(15_000);
    const session = await startHostShell(backend.baseUrl);
    expect(session.session_id).toBeTruthy();
    expect(session.agent_id).toBe("_host_shell");
    expect(session.running).toBe(true);
    expect(session.cmd.length).toBeGreaterThan(0);

    const stream = await openHostShellStream(backend.baseUrl, session.session_id);
    try {
      stream.ws.send(new TextEncoder().encode("echo HELLO_FROM_HOST_SHELL\n"));
      await waitFor(
        () => stream.getOutput().includes("HELLO_FROM_HOST_SHELL"),
        8_000,
        "shell echo",
      );
      await stopHostShell(backend.baseUrl, session.session_id);
      await waitFor(() => stream.hasExited(), 5_000, "legacy shell exit");
    } finally {
      stream.ws.close();
    }
  });

  test("is idempotent per client while distinct clients get independent PTYs", async ({
    backend,
  }) => {
    test.setTimeout(20_000);
    const clientA = "11111111-1111-4111-8111-111111111111";
    const clientB = "22222222-2222-4222-8222-222222222222";
    const first = await startHostShell(backend.baseUrl, clientA);
    const sameClient = await startHostShell(backend.baseUrl, clientA);
    const otherClient = await startHostShell(backend.baseUrl, clientB);
    expect(sameClient.session_id).toBe(first.session_id);
    expect(otherClient.session_id).not.toBe(first.session_id);
    expect(otherClient.agent_id).toBe("_host_shell");

    const streamA = await openHostShellStream(backend.baseUrl, first.session_id);
    const streamB = await openHostShellStream(backend.baseUrl, otherClient.session_id);
    try {
      streamA.ws.send(new TextEncoder().encode("echo CLIENT_A_MARKER\n"));
      streamB.ws.send(new TextEncoder().encode("echo CLIENT_B_MARKER\n"));
      await waitFor(() => streamA.getOutput().includes("CLIENT_A_MARKER"), 8_000, "client A");
      await waitFor(() => streamB.getOutput().includes("CLIENT_B_MARKER"), 8_000, "client B");
      expect(streamA.getOutput()).not.toContain("CLIENT_B_MARKER");
      expect(streamB.getOutput()).not.toContain("CLIENT_A_MARKER");

      await stopHostShell(backend.baseUrl, first.session_id);
      await waitFor(() => streamA.hasExited(), 5_000, "client A exit");
      streamB.ws.send(new TextEncoder().encode("echo CLIENT_B_AFTER_A_STOP\n"));
      await waitFor(
        () => streamB.getOutput().includes("CLIENT_B_AFTER_A_STOP"),
        8_000,
        "client B after A stop",
      );
      expect(streamB.hasExited()).toBe(false);
    } finally {
      streamA.ws.close();
      streamB.ws.close();
      await stopHostShell(backend.baseUrl, first.session_id);
      await stopHostShell(backend.baseUrl, otherClient.session_id);
    }
  });

  test("rejects malformed client IDs and preserves omitted-client compatibility", async ({
    backend,
  }) => {
    const invalid = await fetch(`${backend.baseUrl}/api/v1/host-shell/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cols: 80, rows: 24, client_id: "not-a-uuid" }),
    });
    expect(invalid.status).toBe(400);

    const first = await startHostShell(backend.baseUrl);
    const second = await startHostShell(backend.baseUrl);
    expect(second.session_id).toBe(first.session_id);
    await stopHostShell(backend.baseUrl, first.session_id);
  });
});
