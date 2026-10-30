import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture, summarizeGatewayTraffic } from "./ws-traffic";

export async function verifyCanonicalStream(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  info: TestInfo,
) {
  const id = randomUUID();
  const release = path.join(backend.tmpDir, `canonical-receipt-${id}-start`);
  const finish = path.join(backend.tmpDir, `canonical-receipt-${id}-finish`);
  const script = [
    'e2e:message("Canonical warmup marker\\n")',
    `e2e:await_file(${JSON.stringify(release)})`,
    ...Array.from(
      { length: 20 },
      (_, i) => `e2e:message("Canonical live tick ${i}\\n")\ne2e:delay(200)`,
    ),
    `e2e:await_file(${JSON.stringify(finish)})`,
  ].join("\n");
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Canonical receipt stream",
    seed.agentProfileId,
    {
      description: script,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  const traffic = attachGatewayTrafficCapture(page);
  const changes: Array<{
    reset?: boolean;
    base_revision?: string;
    revision?: string;
    operations?: unknown[];
  }> = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framereceived", ({ payload }) => {
      const frame = JSON.parse(payload.toString());
      if (
        frame.action === "session.conversation.changed" &&
        frame.payload.session_id === task.session_id
      )
        changes.push(frame.payload);
    });
  });
  try {
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await expect(
      session
        .activeChat()
        .locator("[data-agent-message-body]")
        .filter({ hasText: "Canonical warmup marker" }),
    ).toBeVisible();
    await expect
      .poll(() =>
        traffic.frames.some(
          (frame) =>
            frame.direction === "received" && frame.action === "session.conversation.subscribe",
        ),
      )
      .toBe(true);
    changes.length = 0;
    traffic.frames.length = 0;
    fs.writeFileSync(release, "start");
    await expect
      .poll(async () =>
        (await api.listSessionMessages(task.session_id!)).messages.some(
          (message) =>
            message.author_type === "agent" && message.content.includes("Canonical live tick 19"),
        ),
      )
      .toBe(true);
    await expect(
      session
        .activeChat()
        .locator("[data-agent-message-body]")
        .filter({ hasText: "Canonical live tick 19" }),
    ).toBeVisible();
    const history = await api.listSessionMessages(task.session_id!);
    expect(
      history.messages.some(
        (message) =>
          message.metadata?.durable_delivery === true &&
          message.content.includes("Canonical live tick 19"),
      ),
    ).toBe(true);
    const diagnosticDir = path.resolve(
      process.cwd(),
      "../../.kandev/diagnostics/dense-workbench-performance",
    );
    fs.mkdirSync(diagnosticDir, { recursive: true });
    fs.writeFileSync(
      path.join(diagnosticDir, `canonical-stream-${Date.now()}.json`),
      JSON.stringify({ changes, traffic: summarizeGatewayTraffic(traffic.frames) }, null, 2),
    );
    await info.attach("canonical-stream-delivery", {
      body: JSON.stringify({ changes, traffic: summarizeGatewayTraffic(traffic.frames) }, null, 2),
      contentType: "application/json",
    });
    expect(changes.length).toBeGreaterThan(0);
    expect(changes.filter((change) => change.reset)).toEqual([]);
    expect(changes.every((change) => change.base_revision && change.revision)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    fs.writeFileSync(release, "start");
    fs.writeFileSync(finish, "finish");
    await new SessionPage(page).waitForChatIdle({ timeout: 30_000 });
  }
}
