// Routing: /t/{taskId} (task-keyed). File name starts with "mobile-" so it
// runs on the mobile-chrome Playwright project (Pixel 5 emulation).
//
// These tests model real mobile user flows: every action is a button tap or
// an OS-keyboard keystroke. We never poke React state directly. Verification
// is a mix of (a) WS frame capture — a passive observer of `shell.input`
// requests, and (b) terminal buffer text, which is what the user sees.
import { type Page, type Locator, expect as baseExpect } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { MobileTerminalKeybarPage } from "../../pages/mobile-terminal-keybar-page";
import { attachShellInputCapture, type ShellInputFrame } from "../../helpers/ws-capture";

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });
  return session;
}

async function switchToTerminalPanel(testPage: Page): Promise<void> {
  await testPage.getByRole("button", { name: "Terminal" }).tap();
}

/**
 * Wait for the mobile shell to be ready by tailing xterm's buffer until it has
 * any content (a prompt is enough). Mobile mounts the terminal lazily on tab
 * switch so this can take longer than desktop.
 */
async function waitForShellReady(testPage: Page, timeout = 45_000): Promise<void> {
  await expect
    .poll(() => readTerminalBuffer(testPage).then((b) => b.length > 0), {
      timeout,
      message: "Waiting for mobile terminal shell to connect",
    })
    .toBe(true);
}

/**
 * Bring up the OS keyboard the way a user would: tap inside the terminal so
 * xterm focuses its hidden textarea.
 */
async function focusTerminalForTyping(session: SessionPage): Promise<void> {
  await session.terminal.locator(".xterm").click();
}

/**
 * Drives the page's `visualViewport` the way the OS would when the keyboard
 * opens or the user scrolls with it open. Pure event-firing — nothing about
 * the keybar's behavior is touched.
 */
async function simulateKeyboardOpen(testPage: Page, height: number): Promise<void> {
  await testPage.evaluate((px) => {
    const vv = window.visualViewport;
    if (!vv) return;
    Object.defineProperty(vv, "height", { configurable: true, value: window.innerHeight - px });
    vv.dispatchEvent(new Event("resize"));
  }, height);
}

async function simulateViewportScroll(testPage: Page, offsetTop: number): Promise<void> {
  await testPage.evaluate((y) => {
    const vv = window.visualViewport;
    if (!vv) return;
    Object.defineProperty(vv, "offsetTop", { configurable: true, value: y });
    vv.dispatchEvent(new Event("scroll"));
  }, offsetTop);
}

function frameMatching(
  frames: ShellInputFrame[],
  predicate: (f: ShellInputFrame) => boolean,
): ShellInputFrame | undefined {
  return frames.find(predicate);
}

async function expectFrame(
  frames: ShellInputFrame[],
  data: string,
  timeout = 5_000,
): Promise<void> {
  await baseExpect
    .poll(() => frameMatching(frames, (f) => f.data === data), {
      timeout,
      message: `Expected shell.input frame with data ${JSON.stringify(data)}`,
    })
    .toBeTruthy();
}

async function readTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    const xtermEl = panel?.querySelector(".xterm");
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

async function inlineStyleProp(loc: Locator, prop: "top" | "bottom"): Promise<string> {
  return loc.evaluate((el, p) => (el as HTMLElement).style[p], prop);
}

test.describe("Mobile terminal key-bar — user flows", () => {
  test.describe.configure({ retries: 1 });

  test("user cancels a long-running command with the ^C button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Keybar ^C cancels");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await waitForShellReady(testPage);
    await focusTerminalForTyping(session);

    // User types a command on the OS keyboard, then runs it.
    await testPage.keyboard.type("sleep 30");
    await testPage.keyboard.press("Enter");

    // User taps the dedicated ^C button to abort.
    await keybar.ctrlC.tap();

    // Shells (bash/zsh) echo "^C" when they receive SIGINT — an end-to-end
    // signal that the WS round-trip and shell forwarding both work.
    await session.expectTerminalHasText("^C");
  });

  test("user clears the screen with Ctrl + (OS keyboard) L", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Ctrl+L");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);
    await focusTerminalForTyping(session);

    // User taps Ctrl on the bar — visible state change.
    await keybar.ctrl.tap();
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "true");

    // User types "l" on their OS keyboard. The transform applies a Ctrl chord
    // to whatever single letter comes through xterm's onData.
    await testPage.keyboard.press("l");

    // The wire saw \x0c (form feed = Ctrl+L), and Ctrl auto-released.
    await expectFrame(frames, "\x0c");
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  });

  test("user double-taps Ctrl for sticky and chords A then E without re-tapping", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Keybar sticky Ctrl");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);
    await focusTerminalForTyping(session);

    // Two taps → sticky.
    await keybar.ctrl.tap();
    await keybar.ctrl.tap();
    await expect(keybar.ctrl).toHaveAttribute("data-sticky", "true");

    // Multiple chords without re-arming.
    await testPage.keyboard.press("a");
    await testPage.keyboard.press("e");

    await expectFrame(frames, "\x01"); // Ctrl+A
    await expectFrame(frames, "\x05"); // Ctrl+E
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "true");

    // Third tap clears the latch entirely.
    await keybar.ctrl.tap();
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
    await expect(keybar.ctrl).not.toHaveAttribute("data-sticky", "true");
  });

  test("user latches Shift then taps Tab on the bar — emits reverse-tab CSI Z", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Shift+Tab");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);

    await keybar.shift.tap();
    await expect(keybar.shift).toHaveAttribute("aria-pressed", "true");

    await keybar.tap("tab");

    await expectFrame(frames, "\x1b[Z");
    await expect(keybar.shift).toHaveAttribute("aria-pressed", "false");
  });

  test("user presses an OS-keyboard letter while no modifier is active — passes through verbatim", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Keybar passthrough");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);
    await focusTerminalForTyping(session);

    await testPage.keyboard.press("c");

    // No modifier → just the letter.
    await expectFrame(frames, "c");
    // No frame for \x03 (which would mean Ctrl was incorrectly applied).
    expect(frameMatching(frames, (f) => f.data === "\x03")).toBeUndefined();
  });

  test("first Ctrl tap is visibly active (aria-pressed + ring class) — not just a faint shade", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Ctrl visual");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await keybar.ctrl.tap();

    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "true");
    const klass = (await keybar.ctrl.getAttribute("class")) ?? "";
    expect(klass).toContain("ring-");
  });

  test("no in-bar letter buttons exist — even with Ctrl latched", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar no letters");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await keybar.ctrl.tap();

    for (const letter of ["a", "c", "d", "z"]) {
      await expect(testPage.getByTestId(`keybar-key-letter-${letter}`)).toHaveCount(0);
    }
  });

  test("bar is hidden on Chat, Files, Plan, and Changes panels", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar hidden non-terminal");
    const keybar = new MobileTerminalKeybarPage(testPage);

    // Default panel (chat) — hidden.
    await expect(keybar.root).not.toBeVisible();

    for (const panel of ["Files", "Plan", "Changes"] as const) {
      await testPage.getByRole("button", { name: panel }).tap();
      await expect(keybar.root).not.toBeVisible();
    }
  });

  test("user navigates command history with the ↑ button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar arrow up");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);

    await keybar.tap("up");

    await expectFrame(frames, "\x1b[A");
  });

  test("user taps Esc — sends \\x1b (vim-style insert-mode exit)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Esc");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);

    await keybar.tap("esc");

    await expectFrame(frames, "\x1b");
  });

  test("symbol keys (| ~ / - _) all reach the wire on tap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar symbols");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });
    await waitForShellReady(testPage);

    for (const [id, ch] of [
      ["pipe", "|"],
      ["tilde", "~"],
      ["slash", "/"],
      ["dash", "-"],
      ["underscore", "_"],
    ] as const) {
      await keybar.tap(id);
      await expectFrame(frames, ch);
    }
  });

  test("bar switches to top-anchored positioning when the OS keyboard opens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar position keyboard");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    // Closed: bottom-anchored.
    expect(await inlineStyleProp(keybar.root, "bottom")).not.toBe("auto");

    // Opening the keyboard switches the bar to top-anchored — the iOS Safari
    // fix for `position: fixed; bottom:` drift during visual-viewport scroll.
    await simulateKeyboardOpen(testPage, 300);

    await expect
      .poll(() => inlineStyleProp(keybar.root, "bottom"), { timeout: 3_000 })
      .toBe("auto");
    expect(await inlineStyleProp(keybar.root, "top")).not.toBe("");
  });

  test("with the keyboard open, the bar tracks the visual viewport on scroll", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar position scroll");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await simulateKeyboardOpen(testPage, 300);
    const initialTop = await inlineStyleProp(keybar.root, "top");
    expect(initialTop).not.toBe("");

    // User scrolls the page — visualViewport.offsetTop changes.
    await simulateViewportScroll(testPage, 80);

    await expect
      .poll(() => inlineStyleProp(keybar.root, "top"), { timeout: 3_000 })
      .not.toBe(initialTop);
  });

  test("terminal panel reserves bottom space so the bar doesn't cover content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar terminal padding");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    const panel = testPage.getByTestId("terminal-panel");
    const padBefore = await panel.evaluate(
      (el) => parseFloat(getComputedStyle(el).paddingBottom) || 0,
    );
    expect(padBefore).toBeGreaterThan(0);

    // Open the keyboard and the panel pads further so its bottom matches the
    // bar's new top edge instead of being eaten by the keyboard.
    await simulateKeyboardOpen(testPage, 300);

    await expect
      .poll(() => panel.evaluate((el) => parseFloat(getComputedStyle(el).paddingBottom) || 0), {
        timeout: 3_000,
      })
      .toBeGreaterThan(padBefore);
  });

  test("every key button has a non-empty aria-label (a11y)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar a11y");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    const labels = await testPage
      .locator('[data-testid^="keybar-key-"]')
      .evaluateAll((nodes) => nodes.map((n) => n.getAttribute("aria-label") ?? ""));
    expect(labels.length).toBeGreaterThan(5);
    for (const label of labels) {
      expect(label.length).toBeGreaterThan(0);
    }
  });
});
