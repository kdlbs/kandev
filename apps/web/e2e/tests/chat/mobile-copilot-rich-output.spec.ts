import { test, expect } from "../../fixtures/test-base";
import {
  expectCopilotRichOutput,
  openCopilotRichOutputSession,
  seedCopilotRichOutputMessages,
  waitForCopilotRichOutputMessage,
} from "./copilot-rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.18, AC-AGENTS-AGENT-RICH-OUTPUT-002.6
test("keeps Copilot rich output within the phone transcript", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session, sessionId, ws } = await openCopilotRichOutputSession({
    page: testPage,
    apiClient,
    seedData,
    title: "Copilot rich output phone",
  });
  const reportAdded = waitForCopilotRichOutputMessage(ws, sessionId);
  const ids = await seedCopilotRichOutputMessages(apiClient, sessionId);

  await reportAdded;
  let report = await expectCopilotRichOutput(session, ids);
  const reportBox = await report.boundingBox();
  expect(reportBox).not.toBeNull();
  expect(reportBox!.x).toBeGreaterThanOrEqual(0);
  expect(reportBox!.x + reportBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);

  await testPage.reload();
  await session.waitForLoad();
  report = await expectCopilotRichOutput(session, ids);
  const replayedBox = await report.boundingBox();
  expect(replayedBox).not.toBeNull();
  expect(replayedBox!.x + replayedBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
