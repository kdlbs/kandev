import { test } from "../../fixtures/test-base";
import {
  exerciseSSHLayout,
  exerciseSSHCollection,
  exerciseSSHIdentityRemoval,
} from "../../helpers/ssh-management-layout";
test("phone SSH overview and edit", async ({ backend, testPage }) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHLayout(testPage, backend.baseUrl, true);
  } finally {
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

test("SSH identity removal uses confirmation and restores useful focus", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHIdentityRemoval(testPage, backend.baseUrl, true);
  } finally {
    await testPage.close();
    await release();
  }
});
