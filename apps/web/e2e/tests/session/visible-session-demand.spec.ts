import { expect, test } from "../../fixtures/test-base";
import type { GatewayTrafficFrame } from "../../helpers/ws-traffic";
import {
  captureJourneyMetadata,
  reconnectJourneyGateway,
  setJourneyChatSplit,
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

test("only visible sibling chats own rich session streams and drafts survive tab switches", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await apiClient.createTask(seedData.workspaceId, "Visible session demand", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const sessionIds: string[] = [];
  for (let index = 0; index < 8; index += 1) {
    const seeded = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `visible-demand-${index}-${task.id}`,
      agentProfileId: seedData.agentProfileId,
      startedAt: `2026-01-01T00:00:${String(index).padStart(2, "0")}Z`,
    });
    sessionIds.push(seeded.session_id);
  }

  const metadata = await captureJourneyMetadata(testPage);
  const capture = attachGatewayTrafficCapture(testPage);
  await testPage.goto(`/t/${task.id}?sessionId=${sessionIds[0]}`);
  const firstTab = testPage.getByTestId(`session-tab-${sessionIds[0]}`);
  const secondTab = testPage.getByTestId(`session-tab-${sessionIds[1]}`);
  await expect(firstTab).toBeVisible({ timeout: 15_000 });
  await expect(secondTab).toBeVisible();
  await expect(testPage.getByTestId(`session-tab-${sessionIds[7]}`)).toBeVisible();
  const detailConsumers = await testPage
    .locator('[data-testid="session-chat"][data-detail-active="true"]')
    .evaluateAll((panels) => panels.map((panel) => panel.getAttribute("data-session-id")).sort());
  await test.info().attach("visible-session-demand-detail-state.json", {
    body: JSON.stringify(
      {
        detailConsumers,
        panelVisibility: await testPage
          .locator('[data-testid^="session-tab-"]')
          .evaluateAll((tabs) =>
            tabs.map((tab) => {
              const group = tab.closest(".dv-groupview");
              const panelId = tab.getAttribute("data-testid")?.slice("session-tab-".length);
              const content = group?.querySelector<HTMLElement>(".dv-content-container");
              return {
                panelId,
                groupId: group?.getAttribute("data-group-id"),
                activePanel: group?.querySelector(".dv-tab.dv-active-tab")?.textContent,
                groupDisplay: group ? getComputedStyle(group).display : null,
                contentDisplay: content ? getComputedStyle(content).display : null,
              };
            }),
          ),
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
  expect(detailConsumers).toEqual([sessionIds[0]]);
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "eight mounted sibling tabs should retain only the visible rich stream",
    })
    .toEqual([sessionIds[0]]);

  const firstChat = testPage.locator(".chat-message-list:visible").first();
  await expect(firstChat).toBeVisible();
  const draft = "retain this hidden session draft";
  const editor = testPage.locator(".tiptap.ProseMirror:visible").first();
  await editor.fill(draft);

  await testPage.keyboard.press("Control+Shift+]");
  const activeTabSessionIds = await testPage.locator(".dv-tab.dv-active-tab").evaluateAll((tabs) =>
    tabs.flatMap((tab) => {
      const testId = tab.querySelector<HTMLElement>("[data-testid^='session-tab-']")?.dataset
        .testid;
      return testId ? [testId.slice("session-tab-".length)] : [];
    }),
  );
  expect(activeTabSessionIds).toEqual([sessionIds[7]]);
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "keyboard selection should release the prior stream and acquire one stream",
    })
    .toEqual([sessionIds[7]]);
  await testPage.keyboard.press("Control+Shift+[");
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "reverse keyboard selection should release the sibling stream",
    })
    .toEqual([sessionIds[0]]);
  await expect(testPage.locator(".tiptap.ProseMirror:visible").first()).toHaveText(draft);

  const generations = [];
  for (const tab of [secondTab, firstTab, secondTab, firstTab]) {
    await expect
      .poll(() => Object.values(metadata).every((resource) => resource.active === 0))
      .toBe(true);
    const before = Object.fromEntries(
      Object.entries(metadata).map(([key, resource]) => [key, resource.requests]),
    );
    await tab.click();
    const sessionId = tab === firstTab ? sessionIds[0] : sessionIds[1];
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
  await expect(testPage.locator(".tiptap.ProseMirror:visible").first()).toHaveText(draft);
  await test.info().attach("visible-chat-navigation-generations.json", {
    body: JSON.stringify(generations, null, 2),
    contentType: "application/json",
  });

  await setJourneyChatSplit(testPage, sessionIds[0], sessionIds[1], true);
  await expect
    .poll(() => activeSessionIds(capture.frames), { timeout: 30_000 })
    .toEqual([sessionIds[0], sessionIds[1]].sort());
  await expect(
    testPage.locator('[data-testid="session-chat"][data-detail-active="true"]'),
  ).toHaveCount(2);
  await setJourneyChatSplit(testPage, sessionIds[0], sessionIds[1], false);
  await expect
    .poll(() => activeSessionIds(capture.frames), { timeout: 30_000 })
    .toEqual([sessionIds[0]]);

  const desktopViewport = testPage.viewportSize();
  expect(desktopViewport).not.toBeNull();
  await testPage.setViewportSize({ width: 390, height: 844 });
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
  await expect(testPage.locator(".tiptap.ProseMirror:visible").first()).toHaveText(draft);
  await expect
    .poll(() => activeSessionIds(capture.frames), {
      timeout: 30_000,
      message: "responsive transition should retain one detail stream",
    })
    .toEqual([sessionIds[0]]);

  await testPage.setViewportSize(desktopViewport!);
  await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
  await expect(testPage.locator(".tiptap.ProseMirror:visible").first()).toHaveText(draft);

  await test.info().attach("visible-session-demand.json", {
    body: JSON.stringify(
      {
        mountedSiblingTabs: sessionIds.length,
        visibleSessionIds: activeSessionIds(capture.frames),
        subscribeSessionIds: capture.frames
          .filter((frame) => frame.direction === "sent" && frame.action === "session.subscribe")
          .map((frame) => frame.sessionId),
        unsubscribeSessionIds: capture.frames
          .filter((frame) => frame.direction === "sent" && frame.action === "session.unsubscribe")
          .map((frame) => frame.sessionId),
        draftPreserved: true,
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
  expect(
    Object.values(metadata).every((resource) => resource.peak <= 1 && !resource.statuses[503]),
  ).toBe(true);
  await test.info().attach("visible-chat-metadata-resources.json", {
    body: JSON.stringify(metadata, null, 2),
    contentType: "application/json",
  });
});
