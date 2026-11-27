import { test, expect } from "../../fixtures/test-base";

/**
 * Remote Docker create flow, desktop.
 *
 * The connection card is the SSH executor's, reused with a different test
 * endpoint, so this asserts what is specific to remote Docker: the route
 * exists, the root-authority notice is stated where the profile is
 * configured, and Save stays gated until the host is tested and trusted.
 */
test.describe("remote docker executor create flow", () => {
  test("offers the type from the executors hub", async ({ testPage }) => {
    await testPage.goto("/settings/executors");

    const card = testPage.getByRole("button", { name: /remote docker/i });
    await expect(card.first()).toBeVisible();
  });

  test("states the remote root authority before anything is saved", async ({ testPage }) => {
    await testPage.goto("/settings/executors/new/remote_docker");

    await expect(testPage.getByTestId("remote-docker-authority-notice")).toBeVisible();
    await expect(testPage.getByTestId("ssh-connection-card")).toBeVisible();
  });

  test("gates save until the host is tested and trusted", async ({ testPage }) => {
    await testPage.goto("/settings/executors/new/remote_docker");
    await expect(testPage.getByTestId("ssh-connection-card")).toBeVisible();

    const save = testPage.getByTestId("ssh-save-button").first();
    const testButton = testPage.getByTestId("ssh-test-button").first();

    // Nothing filled in: neither action is available.
    await expect(save).toBeDisabled();
    await expect(testButton).toBeDisabled();

    await testPage.getByTestId("ssh-input-name").fill("build-box");
    await testPage.getByTestId("ssh-input-host").fill("build-box.invalid");

    // A named target can be tested, but save still requires a trusted host
    // key, which only a successful test can produce.
    await expect(testButton).toBeEnabled();
    await expect(save).toBeDisabled();
  });
});
