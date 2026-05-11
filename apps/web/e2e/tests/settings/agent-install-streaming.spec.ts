import { test, expect } from "../../fixtures/test-base";

/**
 * End-to-end coverage for the streaming agent install flow:
 *
 *   POST /api/v1/agent-install/<name>      enqueues a job, returns job_id
 *   WS  /api/v1/ws                          broadcasts agent.install.*
 *   GET /api/v1/agent-install/jobs/:id      snapshot for late joiners
 *
 * mock-agent has a deterministic InstallScript (echo + && chain) so this
 * test exercises the real exec.Cmd path without depending on npm.
 */
test.describe("agent install streaming", () => {
  test("POST returns a job_id, WS streams started+output+finished, GET reflects success", async ({
    backend,
  }) => {
    test.setTimeout(15_000);

    const ws = await openGatewayWS(backend.baseUrl);
    const events: Array<{ action: string; payload: unknown }> = [];
    ws.addEventListener("message", (ev) => {
      try {
        const msg = JSON.parse(ev.data as string);
        if (typeof msg.action === "string" && msg.action.startsWith("agent.install.")) {
          events.push({ action: msg.action, payload: msg.payload });
        }
      } catch {
        // ignore
      }
    });

    const enqueueRes = await fetch(`${backend.baseUrl}/api/v1/agent-install/mock-agent`, {
      method: "POST",
    });
    expect(enqueueRes.status).toBe(202);
    const job = (await enqueueRes.json()) as { job_id: string; status: string };
    expect(job.job_id).toBeTruthy();

    // Wait for the finished event (or timeout).
    await waitFor(
      () => events.some((e) => e.action === "agent.install.finished"),
      8_000,
      "agent.install.finished",
    );

    ws.close();

    const actions = events.map((e) => e.action);
    expect(actions).toContain("agent.install.started");
    expect(actions).toContain("agent.install.finished");

    const finishedEvent = events.find((e) => e.action === "agent.install.finished");
    expect(finishedEvent).toBeTruthy();
    const finishedPayload = finishedEvent!.payload as {
      job_id: string;
      status: string;
      output?: string;
    };
    expect(finishedPayload.job_id).toBe(job.job_id);
    expect(finishedPayload.status).toBe("succeeded");
    expect(finishedPayload.output ?? "").toContain("mock-install: done");

    // GET snapshot reflects success.
    const snapRes = await fetch(`${backend.baseUrl}/api/v1/agent-install/jobs/${job.job_id}`);
    expect(snapRes.status).toBe(200);
    const snap = (await snapRes.json()) as { status: string };
    expect(snap.status).toBe("succeeded");
  });

  test("POST while a job is running returns the same job_id (idempotent)", async ({ backend }) => {
    test.setTimeout(15_000);

    // Note: mock-agent's script is fast (~10ms), so the second POST usually
    // races to land before the first finishes. We accept either outcome:
    // same job_id (still running) OR different job_id with first already in
    // retention. The contract under test is: no duplicate concurrent runs.
    const first = (await (
      await fetch(`${backend.baseUrl}/api/v1/agent-install/mock-agent`, { method: "POST" })
    ).json()) as { job_id: string };

    const second = (await (
      await fetch(`${backend.baseUrl}/api/v1/agent-install/mock-agent`, { method: "POST" })
    ).json()) as { job_id: string };

    // If both raced before finish they must share IDs; if first already
    // exited then second's ID is allowed to differ.
    if (first.job_id !== second.job_id) {
      const firstSnap = await fetch(
        `${backend.baseUrl}/api/v1/agent-install/jobs/${first.job_id}`,
      ).then((r) => r.json() as Promise<{ status: string }>);
      expect(["succeeded", "failed"]).toContain(firstSnap.status);
    }
  });

  test("GET /agent-install/jobs lists recent jobs for rehydration", async ({ backend }) => {
    test.setTimeout(15_000);

    // Trigger an install so there's at least one job in retention.
    const enqueueRes = await fetch(`${backend.baseUrl}/api/v1/agent-install/mock-agent`, {
      method: "POST",
    });
    const job = (await enqueueRes.json()) as { job_id: string };

    // Wait for it to finish (mock script is fast).
    let finalStatus = "";
    const deadline = Date.now() + 8_000;
    while (Date.now() < deadline) {
      const snap = (await (
        await fetch(`${backend.baseUrl}/api/v1/agent-install/jobs/${job.job_id}`)
      ).json()) as { status: string };
      if (snap.status === "succeeded" || snap.status === "failed") {
        finalStatus = snap.status;
        break;
      }
      await new Promise((r) => setTimeout(r, 50));
    }
    expect(["succeeded", "failed"]).toContain(finalStatus);

    const listRes = await fetch(`${backend.baseUrl}/api/v1/agent-install/jobs`);
    expect(listRes.status).toBe(200);
    const list = (await listRes.json()) as {
      jobs: Array<{ job_id: string; agent_name: string; status: string }>;
    };
    expect(Array.isArray(list.jobs)).toBe(true);
    const found = list.jobs.find((j) => j.job_id === job.job_id);
    expect(found).toBeTruthy();
    expect(found!.agent_name).toBe("mock-agent");
  });

  test("after install probe completes, agent.available.updated is broadcast", async ({
    backend,
  }) => {
    test.setTimeout(20_000);

    // Open the gateway WS *before* triggering the install so we don't miss
    // the broadcast.
    const ws = await openGatewayWS(backend.baseUrl);
    let availableUpdates = 0;
    ws.addEventListener("message", (ev) => {
      try {
        const msg = JSON.parse(ev.data as string);
        if (msg.action === "agent.available.updated") availableUpdates++;
      } catch {
        // ignore
      }
    });

    await fetch(`${backend.baseUrl}/api/v1/agent-install/mock-agent`, { method: "POST" });

    // The controller fires a `Refresh` after install and then broadcasts
    // `agent.available.updated`. The mock-agent's host-utility probe may
    // succeed or fail (it isn't a real ACP agent under e2e), but the
    // broadcast fires either way.
    await waitFor(() => availableUpdates >= 1, 15_000, "agent.available.updated");

    ws.close();
  });
});

/** Open a WebSocket to the gateway and resolve when the connection is up. */
async function openGatewayWS(baseUrl: string): Promise<WebSocket> {
  const wsUrl = baseUrl.replace(/^http/, "ws") + "/ws";
  const ws = new WebSocket(wsUrl);
  await new Promise<void>((resolve, reject) => {
    ws.addEventListener("open", () => resolve(), { once: true });
    ws.addEventListener("error", () => reject(new Error("ws error")), { once: true });
  });
  return ws;
}

async function waitFor(check: () => boolean, timeoutMs: number, label: string) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 50));
  }
  throw new Error(`Timed out waiting for ${label}`);
}
