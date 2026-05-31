import { test } from "../../fixtures/test-base";
import { assertWatcherAgentProfileResetsToStepDefault } from "./watcher-profile-default-flow";

test.describe("Linear watcher dialog (mobile)", () => {
  test("resets the agent profile back to the step default", async ({ testPage }) => {
    await assertWatcherAgentProfileResetsToStepDefault(testPage);
  });
});
