import { test } from "../../fixtures/test-base";
import { verifyCopilotLongContext } from "./copilot-profile-long-context-helpers";

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.1, AC-AGENTS-COPILOT-CONTEXT-TIER-001.2
test("saves the Copilot long context launch flag on desktop", async ({
  backend,
  apiClient,
  testPage,
  prCapture,
}) => {
  test.setTimeout(90_000);
  await verifyCopilotLongContext({ backend, apiClient, testPage, prCapture }, false);
});
