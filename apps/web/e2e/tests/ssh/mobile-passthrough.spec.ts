import { devices } from "@playwright/test";
import { test, expect } from "../../fixtures/ssh-test-base";
import { openSSHPassthrough, readPassthroughTrace } from "../../helpers/ssh-passthrough";
import { watchWs } from "../../helpers/causal-waits";
import { MobileTerminalKeybarPage } from "../../pages/mobile-terminal-keybar-page";

test.use({ ...devices["Pixel 5"] });

test("phone input and reconnect use the same remote passthrough process", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  expect(testPage.viewportSize()?.width).toBe(393);
  const ws = watchWs(testPage);
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  const input = state.session.passthroughTerminal.locator("textarea.xterm-helper-textarea");
  await state.session.passthroughTerminal.tap();
  await input.focus();
  // Headless typing does not open an OS keyboard. Match its viewport change.
  await testPage.evaluate(() => {
    const viewport = window.visualViewport;
    if (!viewport) throw new Error("Phone test requires visualViewport");
    Object.defineProperty(viewport, "height", {
      configurable: true,
      value: window.innerHeight - 300,
    });
    viewport.dispatchEvent(new Event("resize"));
  });
  await testPage.keyboard.type("phone follow-up", { delay: 20 });
  const keybar = new MobileTerminalKeybarPage(testPage);
  await expect(keybar.root).toBeVisible();
  await keybar.tap("tab");
  await testPage.keyboard.press("Enter");
  await state.session.expectPassthroughHasText("Processed: phone follow-up");
  expect(
    readPassthroughTrace(seedData, state.workspace).filter(
      (entry) => entry.prompt === "phone follow-up\t",
    ),
  ).toHaveLength(1);
  await testPage.reload();
  await state.session.waitForPassthroughLoad();
  await state.session.waitForPassthroughLoaded();
  await state.session.expectPassthroughHasText("phone follow-up");
  expect(
    readPassthroughTrace(seedData, state.workspace).filter((entry) => entry.kind === "start"),
  ).toHaveLength(1);
  const overflows = await testPage.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflows).toBe(false);
  const screenshot = testInfo.outputPath("ssh-phone-terminal.png");
  await testPage.screenshot({ path: screenshot });
  await testInfo.attach("SSH phone terminal", {
    path: screenshot,
    contentType: "image/png",
  });
  await keybar.ctrlC.tap();
  await state.session.expectPassthroughHasText("Mock Agent (RESUMED)");
  await input.focus();
  await testPage.keyboard.type("phone after interrupt", { delay: 20 });
  await testPage.keyboard.press("Enter");
  await state.session.expectPassthroughHasText("Processed: phone after interrupt");
  const trace = readPassthroughTrace(seedData, state.workspace);
  expect(trace.filter((entry) => entry.kind === "start")).toHaveLength(2);
  expect(trace.filter((entry) => entry.prompt?.includes(state.initialPrompt))).toHaveLength(1);
  await apiClient.stopSession({ session_id: state.task.session_id!, force: true });
  await expect(state.session.recoveryResumeButton()).toBeVisible();
  await expect(keybar.root).toBeHidden();
  const resumed = ws.waitForResponse("session.recover", { timeout: 60_000 });
  await state.session.recoveryResumeButton().tap();
  await resumed;
  await state.session.waitForPassthroughLoaded();
  await state.session.expectPassthroughHasText("Mock Agent (RESUMED)");
  await input.focus();
  await testPage.keyboard.type("phone after explicit resume", { delay: 20 });
  await testPage.keyboard.press("Enter");
  await state.session.expectPassthroughHasText("Processed: phone after explicit resume");
});
