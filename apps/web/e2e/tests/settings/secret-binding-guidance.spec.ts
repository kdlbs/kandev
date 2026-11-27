import { randomUUID } from "node:crypto";
import { test } from "../../fixtures/test-base";
import {
  createSecretFromSettings,
  deleteSecretByName,
  expectSecretBindingGuidance,
} from "./secret-binding-guidance-helpers";

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.13
test("explains where to bind Global and Workspace secrets before and after creation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const suffix = randomUUID();
  const globalName = `E2E Global Binding Guidance ${suffix}`;
  const workspaceName = `E2E Workspace Binding Guidance ${suffix}`;
  const globalValue = `global-secret-value-${suffix}`;
  const workspaceValue = `workspace-secret-value-${suffix}`;

  try {
    await testPage.goto("/settings/general/secrets");
    await expectSecretBindingGuidance(testPage, "global");
    await createSecretFromSettings(testPage, globalName, globalValue);
    await expectSecretBindingGuidance(testPage, "global");

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await expectSecretBindingGuidance(testPage, "workspace");
    await createSecretFromSettings(testPage, workspaceName, workspaceValue);
    await expectSecretBindingGuidance(testPage, "workspace");
  } finally {
    await deleteSecretByName(apiClient, "workspace", workspaceName, seedData.workspaceId);
    await deleteSecretByName(apiClient, "global", globalName);
  }
});
