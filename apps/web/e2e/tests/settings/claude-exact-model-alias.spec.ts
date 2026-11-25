import { test } from "../../fixtures/test-base";
import { verifyClaudeExactModelAliasConflict } from "./claude-exact-model-alias-helpers";

test.describe("Claude exact model alias validation", () => {
  test("keeps a conflicting profile draft and allows a full model ID repair", async ({
    testPage,
    apiClient,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await verifyClaudeExactModelAliasConflict(testPage, apiClient, backend, false, prCapture);
  });
});
