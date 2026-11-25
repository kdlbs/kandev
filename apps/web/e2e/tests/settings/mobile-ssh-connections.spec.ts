import {
  exerciseSSHContextSetup,
  exerciseSSHConcurrentEdit,
  exerciseSSHExplicitSelection,
} from "../../helpers/ssh-context-setup";
import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { exerciseSSHIdentitySetup } from "../../helpers/ssh-identities";

for (const width of [390, 820]) {
  test(`managed SSH touch setup preserves a saved destination at ${width}px`, async ({
    backend,
    testPage,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    try {
      await testPage.setViewportSize({ width, height: 900 });
      const identityName = `Phone key ${randomUUID()}`;
      const response = await fetch(`${backend.baseUrl}/api/v1/ssh/managed/identities`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          request_id: randomUUID(),
          kind: "generate",
          label: identityName,
        }),
      });
      expect(response.ok).toBe(true);
      await testPage.goto("/settings/executors/ssh-connections?tab=connections&new=1");
      await testPage.getByLabel("Name", { exact: true }).fill("Phone destination");
      await testPage.getByLabel("Host", { exact: true }).fill("example.invalid");
      await testPage.getByLabel("SSH user", { exact: true }).fill("deploy");

      const picker = testPage.getByRole("button", { name: "Identity", exact: true });
      expect((await requireBox(picker, "identity picker")).height).toBeGreaterThanOrEqual(44);
      await picker.tap();
      const list = testPage.getByRole("listbox", { name: "Identity", exact: true });
      await expect(list).toBeVisible();
      await expect(list).toHaveCSS("overscroll-behavior-y", "contain");
      const identity = list.getByRole("option", { name: identityName, exact: true });
      // Chromium can report a transformed 44px row as 43.99994px.
      const optionHeight = (await requireBox(identity, "identity option")).height;
      expect(Math.round(optionHeight * 100) / 100).toBeGreaterThanOrEqual(44);
      await identity.tap();
      await expect(list).not.toBeVisible();

      await testPage.getByRole("button", { name: "Save changes", exact: true }).tap();
      await expect(testPage).toHaveURL(/id=/);
      await testPage.getByRole("button", { name: "Edit", exact: true }).click();
      await testPage.getByText("Use a bastion", { exact: true }).click();
      await testPage.getByRole("button", { name: "Create bastion", exact: true }).tap();
      await expect(testPage).toHaveURL(/return_connection=/);
      await testPage.getByRole("button", { name: "Return to destination", exact: true }).tap();
      await testPage.getByRole("button", { name: "Edit", exact: true }).click();
      await expect(testPage.getByLabel("Host", { exact: true })).toHaveValue("example.invalid");
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      await testPage.close();
      await release();
    }
  });
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.1, 001.2, 004.3, 004.4
test("phone managed SSH identity creation, protected import, and cancellation", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHIdentitySetup(testPage, backend.baseUrl, true);
  } finally {
    await testPage.close();
    await release();
  }
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
test("creates an identity in context and returns to the executor draft", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHContextSetup(testPage, true);
  } finally {
    await testPage.close();
    await release();
  }
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
test("explicit connection selection requires a new executor check (UI fixtures)", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHExplicitSelection(testPage, true);
  } finally {
    await testPage.close();
    await release();
  }
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
test("reconciles a changed executor after setup (UI fixtures)", async ({ backend, testPage }) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHConcurrentEdit(testPage);
  } finally {
    await testPage.close();
    await release();
  }
});
