import type { Page, WebSocketRoute } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { MobileTerminalKeybarPage } from "../../pages/mobile-terminal-keybar-page";
import { dwell } from "../../helpers/causal-waits";
import { closeQuickTerminalTab } from "../terminal/terminal-test-helpers";

type InputFrame = { url: string; data: string };

function observeInput(page: Page) {
  const frames: InputFrame[] = [];
  const shellRequests: string[] = [];
  page.on("websocket", (socket) => {
    socket.on("framesent", ({ payload }) => {
      const url = new URL(socket.url());
      const data = typeof payload === "string" ? payload : payload.toString("utf8");
      if (
        url.pathname.startsWith("/terminal/") ||
        url.pathname.startsWith("/api/v1/agent-login/sessions/")
      )
        frames.push({ url: socket.url(), data });
      else if (data.includes('"shell.input"')) shellRequests.push(data);
    });
  });
  return { frames, shellRequests };
}

async function openPassthrough(page: Page, api: ApiClient, seed: SeedData, title: string) {
  const { agents } = await api.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("mock-agent missing");
  const profile = await api.createAgentProfile(agent.id, title, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
  const task = await api.createTaskWithAgent(seed.workspaceId, title, profile.id, {
    description: "initial navigation prompt",
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  if (!task.session_id) throw new Error("expected passthrough session");
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForPassthroughLoad();
  await session.waitForPassthroughLoaded();
  await session.expectPassthroughHasText("Processed:");
  await expect
    .poll(
      async () => {
        const { sessions } = await api.listTaskSessions(task.id);
        return sessions.find((item) => item.id === task.session_id)?.state;
      },
      { timeout: 30_000, message: "Waiting for the initial passthrough turn to finish" },
    )
    .toBe("WAITING_FOR_INPUT");
  return { task, session, keybar: new MobileTerminalKeybarPage(page) };
}

// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.2
// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.10, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.11
test("phone passthrough keys deliver once to the agent without a task-shell request", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 375, height: 812 });
  const capture = observeInput(testPage);
  const { task, session, keybar } = await openPassthrough(
    testPage,
    apiClient,
    seedData,
    "Mobile navigation bytes",
  );
  await expect(keybar.root).toHaveCount(1);
  await expect(keybar.root).toBeVisible();
  const keyStrip = keybar.root.getByTestId("mobile-terminal-keybar-scroller");
  const stripBox = await keyStrip.boundingBox();
  expect(stripBox).not.toBeNull();
  const barBox = await keybar.root.boundingBox();
  expect(barBox?.height).toBe(58);
  const stripPadding = await keyStrip.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      left: Number.parseFloat(style.paddingLeft),
      right: Number.parseFloat(style.paddingRight),
      top: Number.parseFloat(style.paddingTop),
      bottom: Number.parseFloat(style.paddingBottom),
    };
  });
  for (const padding of Object.values(stripPadding)) {
    expect(padding).toBeGreaterThanOrEqual(2);
  }
  expect(await keyStrip.evaluate((element) => element.scrollLeft)).toBe(0);
  const initialKeys = await Promise.all(
    ["esc", "up", "down", "left", "right", "enter", "tab"].map(async (id) => {
      const key = keybar.root.getByTestId(`keybar-key-${id}`);
      const exists = (await key.count()) === 1;
      const box = exists ? await key.boundingBox() : null;
      return {
        id,
        exists,
        contained:
          box !== null &&
          box.x >= stripBox!.x &&
          box.x + box.width <= stripBox!.x + stripBox!.width &&
          box.y >= stripBox!.y &&
          box.y + box.height <= stripBox!.y + stripBox!.height,
        width: box?.width ?? 0,
        height: box?.height ?? 0,
        position: box && {
          x: box.x,
          y: box.y,
          right: box.x + box.width,
          bottom: box.y + box.height,
        },
      };
    }),
  );
  expect.soft(initialKeys).toMatchObject(
    ["esc", "up", "down", "left", "right", "enter", "tab"].map((id) => ({
      id,
      exists: true,
      contained: true,
    })),
  );
  for (const key of initialKeys) {
    if (key.exists) {
      expect.soft(key.width).toBeGreaterThanOrEqual(44);
      expect.soft(key.height).toBeGreaterThanOrEqual(44);
    }
  }
  expect.soft(initialKeys[0].position!.x - stripBox.x).toBeGreaterThanOrEqual(2);
  expect
    .soft(stripBox.x + stripBox.width - initialKeys.at(-1)!.position!.right)
    .toBeGreaterThanOrEqual(2);
  const scrollCue = keybar.root.getByTestId("keybar-scroll-cue");
  const scrollCueCount = await scrollCue.count();
  expect.soft(scrollCueCount).toBe(1);
  if (
    initialKeys.some((key) => !key.exists || !key.contained || key.width < 44 || key.height < 44) ||
    scrollCueCount === 0
  )
    return;
  await expect(scrollCue).toBeVisible();
  expect(await keyStrip.evaluate((element) => element.scrollWidth)).toBeGreaterThan(
    await keyStrip.evaluate((element) => element.clientWidth),
  );
  const before = await apiClient.listSessionMessages(task.session_id!);

  // A raw keyboard prompt proves the same native PTY remains interactive.
  await session.passthroughTerminal.tap();
  await testPage.keyboard.type("raw navigation response");
  capture.frames.length = 0;
  await keybar.tap("enter");
  await expect.poll(() => capture.frames.filter((frame) => frame.data === "\r").length).toBe(1);
  const enterFrame = capture.frames.find((frame) => frame.data === "\r")!;
  const enterUrl = new URL(enterFrame.url);
  expect(enterUrl.pathname).toBe(`/terminal/session/${task.session_id}`);
  expect(enterUrl.searchParams.get("mode")).toBe("agent");
  await session.expectPassthroughHasText("Processed: raw navigation response");

  const trailingKey = keybar.root.getByTestId("keybar-key-underscore");
  await trailingKey.scrollIntoViewIfNeeded();
  expect(await keyStrip.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  const [trailingBox, scrolledStripBox] = await Promise.all([
    trailingKey.boundingBox(),
    keyStrip.boundingBox(),
  ]);
  expect(trailingBox).not.toBeNull();
  expect(trailingBox!.x).toBeGreaterThanOrEqual(scrolledStripBox!.x);
  expect(trailingBox!.x + trailingBox!.width).toBeLessThanOrEqual(
    scrolledStripBox!.x + scrolledStripBox!.width,
  );
  expect(trailingBox!.x - scrolledStripBox!.x).toBeGreaterThanOrEqual(2);
  expect(
    scrolledStripBox!.x + scrolledStripBox!.width - trailingBox!.x - trailingBox!.width,
  ).toBeGreaterThanOrEqual(2);

  for (const [key, data] of [
    ["up", "\x1b[A"],
    ["down", "\x1b[B"],
    ["left", "\x1b[D"],
    ["right", "\x1b[C"],
    ["esc", "\x1b"],
    ["tab", "\t"],
  ]) {
    capture.frames.length = 0;
    await keybar.tap(key);
    await expect.poll(() => capture.frames.filter((frame) => frame.data === data).length).toBe(1);
    const frame = capture.frames.find((item) => item.data === data)!;
    const url = new URL(frame.url);
    expect(url.pathname).toBe(`/terminal/session/${task.session_id}`);
    expect(url.searchParams.get("mode")).toBe("agent");
    await expect(session.passthroughTerminal.locator(".xterm-helper-textarea")).toBeFocused();
    await dwell(
      testPage,
      100,
      "negative-assertion",
      "observe duplicate terminal input after one key tap",
    );
    expect(capture.frames.filter((item) => item.data === data)).toHaveLength(1);
  }
  expect(capture.shellRequests).toHaveLength(0);
  const after = await apiClient.listSessionMessages(task.session_id!);
  expect(after.messages.filter((item) => item.author_type === "user")).toHaveLength(
    before.messages.filter((item) => item.author_type === "user").length,
  );
});

// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.3, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.5
test("phone modifiers target the PTY and composer drafts remain unmodified", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const capture = observeInput(testPage);
  const { task, session, keybar } = await openPassthrough(
    testPage,
    apiClient,
    seedData,
    "Mobile modifier ownership",
  );
  await expect(keybar.root).toBeVisible();

  await keybar.ctrl.tap();
  const toggle = testPage.getByTestId("passthrough-toggle-composer");
  await toggle.tap();
  await expect(keybar.root).toHaveCount(0);
  const editor = testPage.getByTestId("passthrough-composer").locator(".tiptap.ProseMirror");
  await editor.fill("draft remains readable");
  await prCapture.screenshot("passthrough-phone-composer", {
    caption: "Phone composer preserves its draft while raw terminal controls are hidden",
  });
  await toggle.tap();
  await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  await toggle.tap();
  await expect(editor).toHaveText("draft remains readable");
  await testPage.getByTestId("passthrough-composer").getByTestId("submit-message-button").tap();
  await session.expectPassthroughHasText("Processed: draft remains readable");
  await expect(keybar.root).toBeVisible();
  const { messages } = await apiClient.listSessionMessages(task.session_id!);
  expect(
    messages.filter(
      (item) => item.author_type === "user" && item.content === "draft remains readable",
    ),
  ).toHaveLength(1);
  expect(capture.shellRequests).toHaveLength(0);

  await keybar.ctrl.tap();
  await testPage.keyboard.press("a");
  await expect.poll(() => capture.frames.some((frame) => frame.data === "\x01")).toBe(true);
  await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  await keybar.ctrl.tap();
  await keybar.ctrl.tap();
  await testPage.keyboard.press("e");
  await testPage.keyboard.press("a");
  await expect.poll(() => capture.frames.filter((frame) => frame.data === "\x01").length).toBe(2);
  await expect.poll(() => capture.frames.some((frame) => frame.data === "\x05")).toBe(true);
  await expect(keybar.ctrl).toHaveAttribute("data-sticky", "true");
  await keybar.ctrl.tap();
  await keybar.shift.tap();
  await keybar.tap("tab");
  await expect.poll(() => capture.frames.some((frame) => frame.data === "\x1b[Z")).toBe(true);

  capture.frames.length = 0;
  await keybar.ctrlC.tap();
  await expect.poll(() => capture.frames.filter((frame) => frame.data === "\x03").length).toBe(1);
});

async function expectPhoneGeometry(page: Page, keyboardOffset: number) {
  const bar = page.getByTestId("mobile-terminal-keybar");
  const output = page.getByTestId("passthrough-terminal");
  const status = page.getByTestId("passthrough-status-row");
  await expect
    .poll(async () => {
      const box = await status.boundingBox();
      return box ? box.y + box.height : Infinity;
    })
    .toBeLessThanOrEqual(page.viewportSize()!.height - keyboardOffset + 1);
  const [barBox, outputBox] = await Promise.all([bar.boundingBox(), output.boundingBox()]);
  expect(outputBox!.y + outputBox!.height).toBeLessThanOrEqual(barBox!.y + 1);
  for (const id of ["ctrl", "shift", "esc", "tab", "up", "down", "left", "right", "enter"]) {
    const key = bar.getByTestId(`keybar-key-${id}`);
    await key.scrollIntoViewIfNeeded();
    const box = await key.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    await expect(key).toHaveAttribute("aria-label", /\S/);
    expect(
      await key.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        );
      }),
    ).toBe(true);
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.4, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.7, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8
test("phone keys clear the keyboard and reset when leaving the agent surface", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { keybar } = await openPassthrough(
    testPage,
    apiClient,
    seedData,
    "Mobile navigation geometry",
  );
  await expect(keybar.root).toBeVisible();
  await expectPhoneGeometry(testPage, 0);
  await prCapture.screenshot("passthrough-phone-keys", {
    caption: "Phone passthrough terminal with inline navigation keys",
  });
  await testPage.evaluate(() => {
    const viewport = window.visualViewport!;
    Object.defineProperty(viewport, "height", {
      configurable: true,
      value: window.innerHeight - 300,
    });
    viewport.dispatchEvent(new Event("resize"));
  });
  await expectPhoneGeometry(testPage, 300);
  await keybar.tap("tab");
  await expect(
    testPage.getByTestId("passthrough-terminal").locator(".xterm-helper-textarea"),
  ).toBeFocused();
  await prCapture.screenshot("passthrough-phone-keyboard", {
    caption: "Passthrough keys and status controls clear keyboard occlusion",
  });
  await testPage.evaluate(() => {
    const viewport = window.visualViewport!;
    Object.defineProperty(viewport, "height", { configurable: true, value: window.innerHeight });
    viewport.dispatchEvent(new Event("resize"));
  });
  await keybar.ctrl.tap();
  const nav = testPage.getByTestId("session-mobile-bottom-nav");
  await nav.getByRole("button", { name: "Plan", exact: true }).tap();
  await expect(keybar.root).toHaveCount(0);
  await nav.getByRole("button", { name: "Chat", exact: true }).tap();
  await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  await testPage.setViewportSize({ width: 767, height: 850 });
  await expect(keybar.root).toBeVisible();
  await expectPhoneGeometry(testPage, 0);
  await testPage.setViewportSize({ width: 768, height: 850 });
  await expect(keybar.root).toHaveCount(0);
});

// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.6
test("phone input resumes on a replacement socket after connection loss", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  let clientSocket: WebSocketRoute | null = null;
  let connections = 0;
  await testPage.routeWebSocket(/\/terminal\/session\/.*mode=agent/, (socket) => {
    clientSocket = socket;
    const server = socket.connectToServer();
    socket.onClose(() => server.close());
    connections += 1;
  });
  const capture = observeInput(testPage);
  const { session, keybar } = await openPassthrough(
    testPage,
    apiClient,
    seedData,
    "Mobile socket replacement",
  );
  await expect(keybar.root).toBeVisible();
  const baseline = connections;
  await keybar.ctrl.tap();
  await clientSocket!.close({ code: 1011, reason: "test connection loss" });
  await expect.poll(() => connections).toBeGreaterThan(baseline);
  await expect(keybar.root).toBeVisible();
  await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  await session.passthroughTerminal.tap();
  await testPage.keyboard.type("reconnected raw input");
  await testPage.keyboard.press("Enter");
  await session.expectPassthroughHasText("Processed: reconnected raw input");
  capture.frames.length = 0;
  await keybar.ctrlD.tap();
  await expect.poll(() => capture.frames.filter((frame) => frame.data === "\x04").length).toBe(1);
  expect(capture.shellRequests).toHaveLength(0);
});

// @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.3, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.6
test("Quick Terminal keeps Ctrl when the background agent reconnects", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  let clientSocket: WebSocketRoute | null = null;
  let connections = 0;
  await testPage.routeWebSocket(/\/terminal\/session\/.*mode=agent/, (socket) => {
    clientSocket = socket;
    const server = socket.connectToServer();
    socket.onClose(() => server.close());
    connections += 1;
  });
  const capture = observeInput(testPage);
  const { keybar } = await openPassthrough(
    testPage,
    apiClient,
    seedData,
    "Quick Terminal modifier ownership",
  );
  await expect(keybar.root).toBeVisible();
  await testPage.getByTestId("app-nav-trigger").tap();
  await testPage.getByTestId("mobile-quick-terminal-button").tap();
  const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
  const tabs = dialog.getByTestId("quick-terminal-tab");
  try {
    const terminal = dialog.getByTestId("quick-terminal-terminal");
    await expect
      .poll(() =>
        terminal.evaluate(
          (element) =>
            (element as HTMLElement & { __xtermReadBuffer?: () => string }).__xtermReadBuffer?.() ??
            "",
        ),
      )
      .not.toBe("");
    const ctrl = dialog.getByTestId("keybar-key-ctrl");
    await ctrl.tap();
    await expect(ctrl).toHaveAttribute("aria-pressed", "true");
    const baseline = connections;
    await clientSocket!.close({ code: 1011, reason: "background connection loss" });
    await expect.poll(() => connections).toBeGreaterThan(baseline);
    await expect(ctrl).toHaveAttribute("aria-pressed", "true");
    capture.frames.length = 0;
    await testPage.keyboard.press("a");
    await expect.poll(() => capture.frames.filter((frame) => frame.data === "\x01").length).toBe(1);
    const frame = capture.frames.find((item) => item.data === "\x01")!;
    expect(new URL(frame.url).pathname).toMatch(
      /^\/api\/v1\/agent-login\/sessions\/[^/]+\/stream$/,
    );
    await expect(ctrl).toHaveAttribute("aria-pressed", "false");
  } finally {
    if (await tabs.count()) await closeQuickTerminalTab(testPage, tabs.last());
  }
  await expect(dialog).toBeHidden();
  await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
});
