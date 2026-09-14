import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { seedSecondaryClarificationTask } from "../../helpers/clarification";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  captureThreadSettings,
  capturePresentation,
  seedThreadPresentation,
  startPresentationThread,
  type ThreadPresentationSettings,
} from "./threads-presentation-helpers";

let original: ThreadPresentationSettings;

async function observeDisclosureMotion(tile: Locator) {
  await tile.evaluate((root) => {
    const samples: { early: number; late: number; duration: number; ciVisible: boolean }[] = [];
    root.setAttribute("data-motion-samples", "[]");
    root.addEventListener("transitionrun", (event) => {
      if ((event as TransitionEvent).propertyName !== "grid-template-rows") return;
      const target = event.target as HTMLElement;
      const animation = target
        .getAnimations()
        .find(
          (item) =>
            item instanceof CSSTransition && item.transitionProperty === "grid-template-rows",
        );
      if (!animation) return;
      const duration = Number(animation.effect!.getComputedTiming().duration);
      const footer = root.querySelector('[data-testid="thread-footer-allocation"]')!;
      const ci = root.querySelector('[data-testid="pr-status-chip"]') as HTMLElement;
      // Sample the browser's real interpolation deterministically, not a sleep.
      animation.pause();
      animation.currentTime = duration * 0.25;
      const early = footer.getBoundingClientRect().height;
      animation.currentTime = duration * 0.75;
      const late = footer.getBoundingClientRect().height;
      const ciVisible =
        !!ci && getComputedStyle(ci).visibility === "visible" && !ci.closest("[inert]");
      animation.finish();
      samples.push({ early, late, duration, ciVisible });
      root.setAttribute("data-motion-samples", JSON.stringify(samples));
    });
  });
}

async function expectDisclosureMotion(tile: Locator, count: number, expanding: boolean) {
  await expect
    .poll(async () => JSON.parse((await tile.getAttribute("data-motion-samples"))!))
    .toHaveLength(count);
  const sample = JSON.parse((await tile.getAttribute("data-motion-samples"))!)[count - 1];
  expect(sample.duration).toBeGreaterThanOrEqual(100);
  expect(sample.duration).toBeLessThanOrEqual(250);
  expect(sample.ciVisible).toBe(true);
  expect(expanding ? sample.late - sample.early : sample.early - sample.late).toBeGreaterThan(5);
}

test.beforeEach(async ({ apiClient, testPage }) => {
  void testPage;
  original = await captureThreadSettings(apiClient);
});

// @covers AC-UI-THREADS-DECK-005.4
test("surfaces a live permission without hover and retains the existing approval path", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Required thread permission",
      seedData.agentProfileId,
      {
        description: "/e2e:permission-flow",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await seedThreadPresentation(apiClient, { layout: "columns", autoHideComposer: true });
    await testPage.mouse.move(0, 0);
    await testPage.goto("/threads");
    const tile = testPage.getByTestId(`thread-column-${task.id}`);
    const approve = tile.getByTestId("permission-approve");
    await expect(approve).toHaveCount(1, { timeout: 30_000 });
    await expect(tile.getByTestId("chat-input-editor")).toBeVisible();
    await expect(tile.getByTestId("collapse-composer")).toBeDisabled();
    await approve.click();
    await waitForLatestSessionDone(apiClient, task.id, 1, "permission approval in Threads");
    await expect(approve).toHaveCount(0);
  } finally {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  }
});
test.afterEach(async ({ apiClient }) => {
  await apiClient.saveUserSettings(original);
});

// @covers AC-UI-THREADS-DECK-005.1, AC-UI-THREADS-DECK-005.2, AC-UI-THREADS-DECK-005.3, AC-UI-THREADS-DECK-005.4, AC-UI-THREADS-DECK-005.7, AC-UI-THREADS-DECK-005.11
test("hides the whole composer except CI and preserves drafts through hover and keyboard reveal", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  await testPage.setViewportSize({ width: 1440, height: 1100 });
  const task = await startPresentationThread(testPage, apiClient, seedData, "A disclosure");
  const sibling = await startPresentationThread(testPage, apiClient, seedData, "B disclosure");
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    owner: "test-owner",
    repo: "test-repo",
    pr_number: 321,
    pr_url: "https://github.com/test-owner/test-repo/pull/321",
    pr_title: "Disclosure PR",
    head_branch: "feature/disclosure",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    checks_state: "success",
    checks_total: 1,
    checks_passing: 1,
  });
  await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
  await testPage.mouse.move(0, 0);
  await testPage.goto("/threads");
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const other = testPage.getByTestId(`thread-column-${sibling.id}`);
  const editor = tile.locator('.tiptap.ProseMirror[contenteditable="true"]');
  await expect(tile.getByTestId("session-chat")).toBeVisible();
  await expect(editor).toHaveCount(1);
  await expect(editor).toBeHidden();
  await expect(tile.getByTestId("pr-status-chip")).toBeVisible();
  await expect(tile.getByTestId("chat-input-area").locator("button:visible")).toHaveCount(1);
  await expect(other.getByTestId("chat-input-area")).toHaveJSProperty("offsetHeight", 0);
  await capturePresentation(testPage, testInfo, "grid-ci-only");
  await observeDisclosureMotion(tile);
  const siblingBounds = await other.boundingBox();
  await tile.locator("header").hover();
  await expect(editor).toBeVisible();
  await expectDisclosureMotion(tile, 1, true);
  await expect(editor).not.toBeFocused();
  expect(await other.boundingBox()).toEqual(siblingBounds);
  await testPage.mouse.move(0, 0);
  await expect(editor).toBeHidden();
  await expectDisclosureMotion(tile, 2, false);
  await tile.focus();
  await expect(editor).toBeVisible();
  await editor.fill("Keep this exact draft");
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
  await expect(tile).toBeFocused();
  await testPage.keyboard.press("Enter");
  await expect(editor).toHaveText("Keep this exact draft");
  await expect(editor).toBeVisible();
  await capturePresentation(testPage, testInfo, "grid-revealed-draft");
  await tile.getByRole("button", { name: "Open task", exact: true }).click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
  await expect(testPage.locator('.tiptap.ProseMirror[contenteditable="true"]')).toBeVisible();
  await expect(testPage.getByTestId("collapse-composer")).toHaveCount(0);
});

// @covers AC-UI-THREADS-DECK-005.3, AC-UI-THREADS-DECK-005.11
test("reveals and collapses immediately with reduced motion", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.emulateMedia({ reducedMotion: "reduce" });
  const task = await startPresentationThread(
    testPage,
    apiClient,
    seedData,
    "Reduced motion composer",
  );
  await seedThreadPresentation(apiClient, { layout: "columns", autoHideComposer: true });
  await testPage.mouse.move(0, 0);
  await testPage.goto("/threads");
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const editor = tile.getByTestId("chat-input-editor");
  await expect(editor).toBeHidden();
  await tile.focus();
  await expect(editor).toBeVisible();
  expect(
    await tile.locator(".thread-composer-disclosure").evaluate((el) => {
      const content = el.querySelector(".thread-composer-content")!;
      return [...el.getAnimations(), ...content.getAnimations()].length;
    }),
  ).toBe(0);
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
  await expect(tile.getByTestId("chat-input-area")).toHaveJSProperty("offsetHeight", 0);
  await testPage.keyboard.press("Enter");
  await expect(editor).toBeVisible();
});

// @covers AC-UI-THREADS-DECK-005.6, AC-UI-THREADS-DECK-005.8
test("preserves history and bottom following while the footer changes height", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  await testPage.setViewportSize({ width: 1250, height: 1000 });
  const task = await startPresentationThread(
    testPage,
    apiClient,
    seedData,
    "A anchored transcript",
  );
  await startPresentationThread(testPage, apiClient, seedData, "B stable sibling");
  await apiClient.seedAgentMessages(task.session_id!, 35, "History marker");
  await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
  await testPage.mouse.move(0, 0);
  await testPage.goto("/threads");
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const list = tile.locator(".chat-message-list");
  const editor = tile.getByTestId("chat-input-editor");
  await expect(editor).toBeHidden();
  await expect(list.getByText("History marker 35", { exact: true })).toHaveCount(1);
  await expect
    .poll(() => list.evaluate((el) => el.scrollHeight - el.clientHeight))
    .toBeGreaterThan(1000);
  await list.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await tile.locator("header").hover();
  await expect(editor).toBeVisible();
  await expect
    .poll(() => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight))
    .toBeLessThan(3);
  await testPage.mouse.move(0, 0);
  await expect(editor).toBeHidden();
  await expect
    .poll(() => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight))
    .toBeLessThan(3);

  await list.evaluate((el) => {
    el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
  });
  const marker = await list.evaluate((el) => {
    const top = el.getBoundingClientRect().top;
    const node = Array.from(el.querySelectorAll("p")).find(
      (p) =>
        p.textContent?.startsWith("History marker") && p.getBoundingClientRect().top > top + 10,
    );
    if (!node) throw new Error("No visible history anchor");
    return { text: node.textContent!, y: node.getBoundingClientRect().top };
  });
  await tile.locator("header").hover();
  await expect(editor).toBeVisible();
  expect((await list.getByText(marker.text, { exact: true }).boundingBox())!.y).toBeCloseTo(
    marker.y,
    0,
  );
  await testPage.mouse.move(0, 0);
  await expect(editor).toBeHidden();
  expect((await list.getByText(marker.text, { exact: true }).boundingBox())!.y).toBeCloseTo(
    marker.y,
    0,
  );
  await tile.focus();
  const toggle = tile.getByTestId("auto-scroll-toggle-button");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", "false");
  const frozenTop = await list.evaluate((el) => el.scrollTop);
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
  expect(await list.evaluate((el) => el.scrollTop)).toBeCloseTo(frozenTop, 0);
  await testPage.keyboard.press("Enter");
  await expect(editor).toBeVisible();
  expect(await list.evaluate((el) => el.scrollTop)).toBeCloseTo(frozenTop, 0);
  await editor.fill(Array.from({ length: 30 }, (_, i) => `Draft line ${i}`).join("\n"));
  expect((await list.boundingBox())!.height).toBeGreaterThanOrEqual(79);
  const footer = tile.getByTestId("thread-footer-allocation");
  const bounds = await footer.boundingBox();
  const tileBounds = await tile.boundingBox();
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(tileBounds!.y + tileBounds!.height);
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
});

// @covers AC-UI-THREADS-DECK-005.4, AC-UI-THREADS-DECK-005.8, AC-UI-THREADS-DECK-005.10
test("only the selected session's required question forces its composer open", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await seedSecondaryClarificationTask(
    apiClient,
    seedData,
    "Selected required action",
  );
  await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
  await testPage.mouse.move(0, 0);
  await testPage.goto(`/threads?taskId=${task.id}&sessionId=${task.primarySessionId}`);
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const editor = tile.getByTestId("chat-input-editor");
  await expect(tile.getByTestId("session-chat")).toHaveAttribute(
    "data-session-id",
    task.primarySessionId,
  );
  await expect(editor).toBeHidden();
  await tile.getByTestId(`thread-session-tab-${task.clarificationSessionId}`).click();
  await expect(tile.getByTestId("clarification-overlay-container")).toBeVisible();
  await testPage.getByTestId("threads-view-picker").focus();
  await testPage.mouse.move(0, 0);
  await dwell(
    testPage,
    450,
    "negative-assertion",
    "selected required action keeps the normal composer open beyond the exit delay",
  );
  await expect(editor).toBeVisible();
  await expect(tile.getByTestId("collapse-composer")).toBeDisabled();
  expect((await tile.locator(".chat-message-list").boundingBox())!.height).toBeGreaterThanOrEqual(
    79,
  );
  await tile.getByTestId(`thread-session-tab-${task.primarySessionId}`).click();
  await testPage.getByTestId("threads-view-picker").focus();
  await testPage.mouse.move(0, 0);
  await expect(editor).toBeHidden();
});

// @covers AC-UI-THREADS-DECK-005.4, AC-UI-THREADS-DECK-005.5
test("preserves a rejected draft and native cancellation feedback after retry", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const task = await startPresentationThread(
    testPage,
    apiClient,
    seedData,
    "Recoverable composer send",
  );
  let reject = true;
  let rejected = 0;
  await testPage.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      const frame = JSON.parse(message.toString());
      if (reject && frame.type === "request" && frame.action === "message.add") {
        reject = false;
        rejected++;
        socket.send(
          JSON.stringify({
            id: frame.id,
            type: "error",
            action: frame.action,
            payload: { code: "INTERNAL_ERROR", message: "Disclosure send rejected" },
          }),
        );
      } else server.send(message);
    });
  });
  await seedThreadPresentation(apiClient, { layout: "columns", autoHideComposer: true });
  await testPage.goto("/threads");
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const editor = tile.getByTestId("chat-input-editor");
  await tile.focus();
  await editor.fill("/slow 8s");
  await tile.getByTestId("submit-message-button").click();
  await expect.poll(() => rejected).toBe(1);
  await expect(editor).toHaveText("/slow 8s");
  await expect(tile.getByTestId("collapse-composer")).toBeEnabled();
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
  await testPage.keyboard.press("Enter");
  await expect(editor).toHaveText("/slow 8s");
  await tile.getByTestId("submit-message-button").click();
  await expect(
    tile.locator(".chat-message-list").getByText("/slow 8s", { exact: true }),
  ).toBeVisible();
  const cancel = tile.getByTestId("cancel-agent-button");
  await expect(cancel).toBeVisible();
  await cancel.click();
  await expect(cancel).toBeDisabled();
  await expect(tile.getByTestId("collapse-composer")).toBeDisabled();
  await expect(cancel).toBeHidden({ timeout: 15_000 });
  await expect(editor).toBeEmpty();
});

// @covers AC-UI-THREADS-DECK-005.3, AC-UI-THREADS-DECK-005.4
test("keeps the native model picker and attachment-only draft available after pointer exit", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await startPresentationThread(testPage, apiClient, seedData, "Composer controls");
  await seedThreadPresentation(apiClient, { layout: "columns", autoHideComposer: true });
  await testPage.goto("/threads");
  const tile = testPage.getByTestId(`thread-column-${task.id}`);
  const editor = tile.getByTestId("chat-input-editor");
  await tile.locator("header").hover();
  const model = tile.getByRole("button", { name: "Session model settings" });
  await model.click();
  const options = testPage.getByRole("listbox");
  await expect(options).toBeVisible();
  await testPage.mouse.move(0, 0);
  await dwell(
    testPage,
    450,
    "negative-assertion",
    "composer must stay open beyond its exit delay while the owned model picker is open",
  );
  await expect(editor).toBeVisible();
  await options.getByRole("option", { name: /Mock Smart/ }).click();
  await expect(model).toContainText("Mock Smart");
  await tile.locator('input[type="file"]').setInputFiles({
    name: "notes.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("Attachment-only draft"),
  });
  await expect(tile.getByText("notes.txt", { exact: false })).toBeVisible();
  await expect(tile.getByTestId("submit-message-button")).toBeEnabled();
  await tile.getByTestId("collapse-composer").click();
  await expect(editor).toBeHidden();
  await testPage.keyboard.press("Enter");
  await expect(tile.getByText("notes.txt", { exact: false })).toBeVisible();
  await tile.getByTestId("submit-message-button").click();
  await expect
    .poll(async () =>
      (await apiClient.listSessionMessages(task.session_id!)).messages.some(
        (message) =>
          message.author_type === "user" &&
          (message.metadata?.attachments as unknown[] | undefined)?.length,
      ),
    )
    .toBe(true);
});
