import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createSecretFromSettings,
  deleteSecretByName,
  expectSecretBindingGuidance,
} from "./secret-binding-guidance-helpers";

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.13
test("keeps Global and Workspace binding guidance readable on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const suffix = randomUUID();
  const globalName = `E2E Mobile Global Binding Guidance ${suffix}`;
  const workspaceName = `E2E Mobile Workspace Binding Guidance ${suffix}`;
  const globalValue = `mobile-global-secret-value-${suffix}`;
  const workspaceValue = `mobile-workspace-secret-value-${suffix}`;

  try {
    await testPage.goto("/settings/general/secrets");
    const globalDescription = await expectSecretBindingGuidance(testPage, "global");
    const emptyGlobalBox = await globalDescription.boundingBox();
    expect(emptyGlobalBox).not.toBeNull();
    expect(emptyGlobalBox!.x).toBeGreaterThanOrEqual(0);
    expect(emptyGlobalBox!.x + emptyGlobalBox!.width).toBeLessThanOrEqual(390);
    expect(emptyGlobalBox!.height).toBeGreaterThan(0);
    await assertNoDocumentHorizontalOverflow(testPage, "empty mobile Global Secrets guidance");
    await createSecretFromSettings(testPage, globalName, globalValue);
    const populatedGlobalDescription = await expectSecretBindingGuidance(testPage, "global");
    const populatedGlobalBox = await populatedGlobalDescription.boundingBox();
    expect(populatedGlobalBox).not.toBeNull();
    expect(populatedGlobalBox!.x).toBeGreaterThanOrEqual(0);
    expect(populatedGlobalBox!.x + populatedGlobalBox!.width).toBeLessThanOrEqual(390);
    expect(populatedGlobalBox!.height).toBeGreaterThan(0);
    await assertNoDocumentHorizontalOverflow(testPage, "populated mobile Global Secrets guidance");

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
    const tabs = testPage.getByTestId("workspace-settings-tabs");
    await expect(tabs).toBeVisible();
    await tabs.getByRole("link", { name: "Secrets", exact: true }).click();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/secrets$`),
    );
    const workspaceDescription = await expectSecretBindingGuidance(testPage, "workspace");
    const emptyWorkspaceBox = await workspaceDescription.boundingBox();
    expect(emptyWorkspaceBox).not.toBeNull();
    expect(emptyWorkspaceBox!.x).toBeGreaterThanOrEqual(0);
    expect(emptyWorkspaceBox!.x + emptyWorkspaceBox!.width).toBeLessThanOrEqual(390);
    expect(emptyWorkspaceBox!.height).toBeGreaterThan(0);
    await assertNoDocumentHorizontalOverflow(testPage, "empty mobile Workspace Secrets guidance");
    await createSecretFromSettings(testPage, workspaceName, workspaceValue);
    const populatedWorkspaceDescription = await expectSecretBindingGuidance(testPage, "workspace");
    const populatedWorkspaceBox = await populatedWorkspaceDescription.boundingBox();
    expect(populatedWorkspaceBox).not.toBeNull();
    expect(populatedWorkspaceBox!.x).toBeGreaterThanOrEqual(0);
    expect(populatedWorkspaceBox!.x + populatedWorkspaceBox!.width).toBeLessThanOrEqual(390);
    expect(populatedWorkspaceBox!.height).toBeGreaterThan(0);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "populated mobile Workspace Secrets guidance",
    );
  } finally {
    await deleteSecretByName(apiClient, "workspace", workspaceName, seedData.workspaceId);
    await deleteSecretByName(apiClient, "global", globalName);
  }
});
