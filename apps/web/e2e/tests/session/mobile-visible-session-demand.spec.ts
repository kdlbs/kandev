import { expect, test } from "../../fixtures/test-base";
import type { GatewayTrafficFrame } from "../../helpers/ws-traffic";
import {
  captureJourneyMetadata,
  reconnectJourneyGateway,
} from "../task/journey-boot-loading-helpers";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";

function activeSessionIds(frames: readonly GatewayTrafficFrame[]): string[] {
  const active = new Set<string>();
  for (const frame of frames) {
    if (frame.direction !== "sent" || !frame.sessionId) continue;
    if (frame.action === "session.subscribe") active.add(frame.sessionId);
    if (frame.action === "session.unsubscribe") active.delete(frame.sessionId);
  }
  return [...active].sort();
}

test("phone keeps one visible session stream while switching sessions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile visible session demand", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const first = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `mobile-visible-first-${task.id}`,
    agentProfileId: seedData.agentProfileId,
    startedAt: "2026-01-01T00:00:00Z",
  });
  const second = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `mobile-visible-second-${task.id}`,
    agentProfileId: seedData.agentProfileId,
    startedAt: "2026-01-01T00:01:00Z",
  });

  const metadata = await captureJourneyMetadata(testPage);
  const capture = attachGatewayTrafficCapture(testPage);
  await testPage.goto(`/t/${task.id}?sessionId=${first.session_id}`);
  const layout = testPage.getByTestId("mobile-task-layout");
  const picker = layout.getByTestId("mobile-sessions-pill");
  await expect(layout).toBeVisible();
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "phone should start one rich session stream",
    })
    .toEqual([first.session_id]);

  await picker.tap();
  await testPage.getByTestId(`mobile-session-row-${second.session_id}`).tap();
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "phone session switch should release the old stream and acquire one new stream",
    })
    .toEqual([second.session_id]);
  const generations = [];
  for (const sessionId of [
    first.session_id,
    second.session_id,
    first.session_id,
    second.session_id,
  ]) {
    await expect
      .poll(() => Object.values(metadata).every((resource) => resource.active === 0))
      .toBe(true);
    const before = Object.fromEntries(
      Object.entries(metadata).map(([key, resource]) => [key, resource.requests]),
    );
    await picker.tap();
    await testPage.getByTestId(`mobile-session-row-${sessionId}`).tap();
    await expect
      .poll(() => activeSessionIds(capture.frames), { timeout: 30_000 })
      .toEqual([sessionId]);
    await expect
      .poll(() => Object.values(metadata).every((resource) => resource.active === 0))
      .toBe(true);
    const newMetadataRequests = Object.fromEntries(
      Object.entries(metadata).map(([key, resource]) => [
        key,
        resource.requests - (before[key] ?? 0),
      ]),
    );
    generations.push({ sessionId, newMetadataRequests });
    expect(Object.values(newMetadataRequests).every((count) => count <= 1)).toBe(true);
  }
  await reconnectJourneyGateway(testPage, capture.frames);
  await expect.poll(() => activeSessionIds(capture.frames)).toEqual([second.session_id]);
  await test.info().attach("visible-chat-navigation-generations.json", {
    body: JSON.stringify(generations, null, 2),
    contentType: "application/json",
  });
  await expect(layout.getByTestId("mobile-sessions-pill")).toBeVisible();
  await test.info().attach("visible-chat-metadata-resources.json", {
    body: JSON.stringify(metadata, null, 2),
    contentType: "application/json",
  });
  expect(
    Object.values(metadata).every((resource) => resource.peak <= 1 && !resource.statuses[503]),
    JSON.stringify(metadata),
  ).toBe(true);
});
