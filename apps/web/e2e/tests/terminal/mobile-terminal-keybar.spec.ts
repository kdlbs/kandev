// Routing: /t/{taskId} (task-keyed). File name starts with "mobile-" so it
// runs on the mobile-chrome Playwright project (Pixel 5 emulation).
import { type Page } from "@playwright/test";
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

async function fakeVisualViewportResize(testPage: Page, shrinkBy: number): Promise<void> {
  await testPage.evaluate((px) => {
    const vv = window.visualViewport;
    if (!vv) return;
    Object.defineProperty(vv, "height", { configurable: true, value: window.innerHeight - px });
    vv.dispatchEvent(new Event("resize"));
  }, shrinkBy);
}

function firstFrameMatching(
  frames: ShellInputFrame[],
  predicate: (f: ShellInputFrame) => boolean,
): ShellInputFrame | undefined {
  return frames.find(predicate);
}

test.describe("Mobile terminal keybar", () => {
  test.describe.configure({ retries: 1 });

  test("renders on the terminal panel and sends Esc via shell.input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Esc");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await keybar.tap("esc");
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x1b"), { timeout: 5_000 })
      .toBeTruthy();
  });

  test("hidden on non-terminal panels", async ({ testPage, apiClient, seedData }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Chat Hidden");
    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).not.toBeVisible();

    // Switch to a non-terminal panel — still hidden.
    await testPage.getByRole("button", { name: "Files" }).tap();
    await expect(keybar.root).not.toBeVisible();
  });

  test("Tab, arrows, Home/End, PgUp/PgDn all map to the right sequences", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Keys");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    const taps: Array<[string, string]> = [
      ["tab", "\t"],
      ["up", "\x1b[A"],
      ["down", "\x1b[B"],
      ["left", "\x1b[D"],
      ["right", "\x1b[C"],
      ["home", "\x01"],
      ["end", "\x05"],
      ["pageup", "\x1b[5~"],
      ["pagedown", "\x1b[6~"],
      ["pipe", "|"],
      ["tilde", "~"],
    ];

    for (const [id, expected] of taps) {
      await keybar.tap(id);
      await expect
        .poll(() => firstFrameMatching(frames, (f) => f.data === expected), {
          timeout: 5_000,
          message: `Expected shell.input with data ${JSON.stringify(expected)} after tapping ${id}`,
        })
        .toBeTruthy();
    }
  });

  test("dedicated Ctrl+C and Ctrl+D buttons send the expected bytes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar CtrlCD");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await keybar.ctrlC.tap();
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x03"), { timeout: 5_000 })
      .toBeTruthy();

    await keybar.ctrlD.tap();
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x04"), { timeout: 5_000 })
      .toBeTruthy();
  });

  test("sticky Ctrl: single tap latches, chord fires, then auto-releases", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Sticky Ctrl");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
    await keybar.ctrl.tap();
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "true");

    // Letter buttons only render while Ctrl is latched.
    await keybar.tap("letter-c");
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x03"), { timeout: 5_000 })
      .toBeTruthy();
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "false");
  });

  test("double-tap Ctrl stays sticky across multiple chords", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { frames } = attachShellInputCapture(testPage);
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Ctrl Sticky Persist");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    await keybar.ctrl.tap();
    await keybar.ctrl.tap();
    await expect(keybar.ctrl).toHaveAttribute("data-sticky", "true");

    await keybar.tap("letter-c");
    await keybar.tap("letter-d");
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x03"), { timeout: 5_000 })
      .toBeTruthy();
    await expect
      .poll(() => firstFrameMatching(frames, (f) => f.data === "\x04"), { timeout: 5_000 })
      .toBeTruthy();
    await expect(keybar.ctrl).toHaveAttribute("aria-pressed", "true");
  });

  test("every key button has a non-empty aria-label", async ({ testPage, apiClient, seedData }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar A11y");
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

  test("bar moves up when visualViewport shrinks (simulated keyboard open)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Viewport");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    const initialBottom = await keybar.root.evaluate((el) => {
      return window.innerHeight - el.getBoundingClientRect().bottom;
    });

    await fakeVisualViewportResize(testPage, 300);

    await expect
      .poll(
        async () =>
          keybar.root.evaluate((el) => window.innerHeight - el.getBoundingClientRect().bottom),
        { timeout: 3_000 },
      )
      .toBeGreaterThan(initialBottom);
  });

  test("Ctrl+C reaches the shell (^C echoes in buffer)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Keybar Ctrl+C shell");
    await switchToTerminalPanel(testPage);

    const keybar = new MobileTerminalKeybarPage(testPage);
    await expect(keybar.root).toBeVisible({ timeout: 10_000 });

    // Mobile shell boot takes longer than desktop because ShellTerminal only
    // mounts after the terminal tab is active. Give it up to 45s.
    await expect
      .poll(() => readTerminalBuffer(testPage).then((b) => b.length > 0), {
        timeout: 45_000,
        message: "Waiting for mobile terminal shell to connect",
      })
      .toBe(true);

    // Focus the terminal so subsequent key presses go to xterm.
    await session.terminal.locator(".xterm").click();

    await keybar.ctrlC.tap();

    // Most shells (bash/zsh) echo "^C" when they receive SIGINT. Asserting
    // on the echo gives us an end-to-end signal that the WS round-trip and
    // backend shell forwarding both work — without relying on precise
    // prompt state, which is flaky on a fresh zsh setup.
    await session.expectTerminalHasText("^C");
  });
});

async function readTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    const xtermEl = panel?.querySelector(".xterm");
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}
