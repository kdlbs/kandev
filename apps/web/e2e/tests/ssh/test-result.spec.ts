import { renameSync } from "node:fs";
import { waitForHttp } from "../../helpers/causal-waits";
import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

test("executor test reports a missing credential and succeeds after it is restored", async ({
  testPage,
  seedData,
}) => {
  const page = new SSHSettingsPage(testPage);
  await page.gotoExisting(seedData.sshExecutorId);
  const identity = seedData.sshTarget.identityFile;
  const hidden = `${identity}.unavailable`;
  renameSync(identity, hidden);
  try {
    const completed = waitForHttp(testPage, "POST", /^\/api\/v1\/ssh\/test$/);
    await page.testButton.click();
    await completed;
    await expect(
      testPage.getByRole("alert").filter({ hasText: "Executor test failed" }),
    ).toBeVisible();
    await expect(page.executorReady).toHaveCount(0);
  } finally {
    renameSync(hidden, identity);
  }
  const completed = waitForHttp(testPage, "POST", /^\/api\/v1\/ssh\/test$/);
  await page.testButton.click();
  await completed;
  await expect(page.executorReady).toBeVisible();
});
