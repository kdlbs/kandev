import { test } from "../../fixtures/test-base";
import { expectBuiltinRichOutput, openBuiltinRichOutput } from "./builtin-rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.13, AC-AGENTS-AGENT-RICH-OUTPUT-001.17,
// AC-AGENTS-AGENT-RICH-OUTPUT-002.6, AC-AGENTS-AGENT-RICH-OUTPUT-002.9
test("renders package-qualified previews and table live and after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await openBuiltinRichOutput({ page: testPage, apiClient, seedData });
  await expectBuiltinRichOutput(testPage, fixture);
  await testPage.reload();
  await fixture.session.waitForLoad();
  await expectBuiltinRichOutput(testPage, fixture);
});
