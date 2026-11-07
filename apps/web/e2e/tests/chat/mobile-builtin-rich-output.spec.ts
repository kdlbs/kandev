import { test } from "../../fixtures/test-base";
import { expectBuiltinRichOutput, openBuiltinRichOutput } from "./builtin-rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.18, AC-UI-KANDEV-MCP-TOOL-RESULTS-001.3
test("renders package-qualified evidence in the phone transcript and after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await openBuiltinRichOutput({ page: testPage, apiClient, seedData });
  await expectBuiltinRichOutput(testPage, fixture, true);
  await testPage.reload();
  await fixture.session.waitForLoad();
  await expectBuiltinRichOutput(testPage, fixture, true);
});
