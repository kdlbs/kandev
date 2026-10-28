import { test, expect } from "../../fixtures/test-base";
import {
  exerciseSSHLayout,
  exerciseSSHCollection,
  exerciseSSHIdentityRemoval,
} from "../../helpers/ssh-management-layout";
for (const width of [767, 768, 1280]) {
  test(`focused SSH overview and edit at ${width}px`, async ({ backend, testPage }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    try {
      await testPage.setViewportSize({ width, height: 900 });
      await exerciseSSHLayout(testPage, backend.baseUrl, width < 768);
    } finally {
      await testPage.close();
      await release();
    }
  });
}

test("coarse tablet SSH overview and edit", async ({ backend, testPage, tabletTestPage }) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await tabletTestPage.setViewportSize({ width: 820, height: 1000 });
    await exerciseSSHLayout(tabletTestPage, backend.baseUrl, true);
  } finally {
    await tabletTestPage.close();
    await testPage.close();
    await release();
  }
});

test("SSH collection distinguishes identical records and fits translations", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHCollection(testPage, backend.baseUrl);
  } finally {
    await testPage.close();
    await release();
  }
});

test("SSH collections recover failed reads without presenting a false empty list", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  let fail = true;
  await testPage.route("**/api/v1/ssh/managed/connections", (route) =>
    route.fulfill(fail ? { status: 503, json: { error: "ssh_operation_failed" } } : { json: [] }),
  );
  try {
    await testPage.goto("/settings/executors/ssh-connections");
    await expect(testPage.getByRole("alert")).toBeVisible();
    await expect(
      testPage.getByText("No connections yet. Save a host to begin setup.", { exact: true }),
    ).toHaveCount(0);
    fail = false;
    const response = testPage.waitForResponse(
      (r) => r.url().endsWith("/ssh/managed/connections") && r.ok(),
    );
    await testPage.getByRole("button", { name: "Reload", exact: true }).click();
    await response;
    await expect(testPage.getByRole("alert")).toHaveCount(0);
    await testPage.goto("/settings/executors/ssh-connections?id=missing");
    await expect(testPage.getByRole("alert")).toBeVisible();
    await testPage.getByRole("button", { name: "Back to connections", exact: true }).click();
    await expect(
      testPage.getByRole("button", { name: "New connection", exact: true }),
    ).toBeVisible();
  } finally {
    await testPage.close();
    await release();
  }
});

test("SSH identity removal uses confirmation and restores useful focus", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHIdentityRemoval(testPage, backend.baseUrl, false);
  } finally {
    await testPage.close();
    await release();
  }
});
