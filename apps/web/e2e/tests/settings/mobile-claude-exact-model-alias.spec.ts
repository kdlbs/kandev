import { test } from "../../fixtures/test-base";
import { verifyClaudeExactModelAliasConflict } from "./claude-exact-model-alias-helpers";

test.describe("Claude exact model alias validation on mobile", () => {
  test("keeps the draft editable by touch and allows a full model ID repair", async ({
    testPage,
    apiClient,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await verifyClaudeExactModelAliasConflict(testPage, apiClient, backend, true, prCapture);
  });
});
