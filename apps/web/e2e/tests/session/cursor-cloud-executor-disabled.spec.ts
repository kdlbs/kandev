import { test, expect } from "../../fixtures/test-base";

test("disabled Cursor Cloud cannot be configured from settings or a direct route", async ({
  testPage,
  backend,
}) => {
  const restore = await backend.useEnv({ KANDEV_FEATURES_CURSOR_CLOUD: "false" });
  try {
    await testPage.goto("/settings/executors");
    await expect(testPage.getByRole("button", { name: /^Worktree/ })).toHaveCount(1);
    await expect(testPage.getByRole("button", { name: /Cursor Cloud/ })).toHaveCount(0);
    await testPage.goto("/settings/executors/new/cursor_cloud");
    await expect(testPage.getByText("Unknown executor type", { exact: true })).toBeVisible();
  } finally {
    await restore();
  }
});
