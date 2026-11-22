import { test } from "../../fixtures/test-base";
import {
  expectCopilotRichOutput,
  openCopilotRichOutputSession,
  seedCopilotRichOutputMessages,
  waitForCopilotRichOutputMessage,
} from "./copilot-rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.1, AC-AGENTS-AGENT-RICH-OUTPUT-001.13,
// AC-AGENTS-AGENT-RICH-OUTPUT-001.17, AC-AGENTS-AGENT-RICH-OUTPUT-002.6, AC-AGENTS-AGENT-RICH-OUTPUT-002.9
test("renders Copilot rich output live and after history reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session, sessionId, ws } = await openCopilotRichOutputSession({
    page: testPage,
    apiClient,
    seedData,
    title: "Copilot rich output desktop",
  });
  const reportAdded = waitForCopilotRichOutputMessage(ws, sessionId);
  const ids = await seedCopilotRichOutputMessages(apiClient, sessionId);

  await reportAdded;
  await expectCopilotRichOutput(session, ids);
  await testPage.reload();
  await session.waitForLoad();
  await expectCopilotRichOutput(session, ids);
});
