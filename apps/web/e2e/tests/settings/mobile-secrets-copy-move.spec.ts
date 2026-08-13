import { test, expect } from "../../fixtures/test-base";
import {
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

const WORKSPACE_VALUE = "e2e-mobile-copy-move-value";
const GLOBAL_VALUE = "e2e-mobile-copy-move-global";

/** Unique per run so retries never collide with leftovers from a failed attempt. */
const runToken = () => `${Date.now()}${Math.random().toString(36).slice(2, 6)}`;

test.describe("mobile-secrets-copy-move", () => {
  test("moves a workspace secret to General and copies one back at phone width", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const sourceName = `E2E Mobile Copy Move ${runToken()}`;
    const movedName = `E2E Mobile Moved ${runToken()}`;
    const copiedName = `E2E Mobile Copied ${runToken()}`;

    // Seed a General secret whose name collides with the max-length target
    // name, so the conflict message (not just the input) is exercised below.
    const longName = "x".repeat(100);
    await testPage.goto("/settings/general/secrets");
    await testPage.getByRole("button", { name: "Add secret", exact: true }).click();
    await testPage.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(longName);
    await testPage.getByPlaceholder("Secret value").fill(GLOBAL_VALUE);
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByText(longName, { exact: true })).toBeVisible();

    // Create a workspace secret through the mobile flow.
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await testPage.getByRole("button", { name: "Add secret", exact: true }).click();
    await testPage.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(sourceName);
    await testPage.getByPlaceholder("Secret value").fill(WORKSPACE_VALUE);
    const secretSave = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(secretSave).toBeEnabled();
    await secretSave.click();
    await expect(testPage.getByText(sourceName, { exact: true })).toBeVisible();

    // Move it to General through the dialog's own controls.
    await testPage.getByRole("button", { name: `Copy or move ${sourceName}` }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("radio", { name: /Move/ }).click();
    await dialog.getByLabel("Name").fill(movedName);

    // The mode option CARDS are the touch targets (the radio indicator itself
    // is the standard 16px control).
    for (const control of [
      dialog.getByLabel("Name"),
      dialog.locator("label").filter({ hasText: "Copy" }),
      dialog.locator("label").filter({ hasText: "Move" }),
      dialog.getByRole("button", { name: "Move", exact: true }),
      dialog.getByRole("button", { name: "Close", exact: true }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile copy/move dialog");

    // Scroll contract: the settings content area is the single scroll owner.
    // The bottom-sheet dialog must not create document scroll or push its
    // actions below the viewport.
    const documentVerticalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollHeight > window.innerHeight,
    );
    expect(documentVerticalOverflow).toBe(false);
    const moveButtonBox = await dialog
      .getByRole("button", { name: "Move", exact: true })
      .boundingBox();
    expect(moveButtonBox).not.toBeNull();
    expect(moveButtonBox!.y + moveButtonBox!.height).toBeLessThanOrEqual(844);

    // Max-length unbroken text must not push any dialog descendant past the
    // sheet's right edge: fill the target name with a value that CONFLICTS
    // with the seeded General secret, so the conflict message is rendered
    // while its wrapping (break-words) is exercised.
    await dialog.getByLabel("Name").fill(longName);
    await expect(dialog.getByText(/already exists in this destination/)).toBeVisible();
    await assertNoDescendantOverflowsRight(dialog, "mobile copy/move dialog with 100-char name");
    await dialog.getByLabel("Name").fill(movedName);

    await dialog.getByRole("button", { name: "Move", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(testPage.getByText(sourceName, { exact: true })).toHaveCount(0);
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);

    // The moved secret now lives in General.
    await testPage.goto("/settings/general/secrets");
    await expect(testPage.getByText(movedName, { exact: true })).toBeVisible();

    // Copy it back to the workspace using the destination picker.
    await testPage.getByRole("button", { name: `Copy or move ${movedName}` }).click();
    const copyDialog = testPage.getByRole("dialog");
    await expect(copyDialog).toBeVisible();
    await copyDialog.getByLabel("Name").fill(copiedName);

    const workspaces = await apiClient.listWorkspaces();
    const workspaceName = workspaces.workspaces.find(
      (workspace) => workspace.id === seedData.workspaceId,
    )?.name;
    expect(workspaceName).toBeTruthy();
    await copyDialog.getByRole("combobox", { name: "Destination" }).click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: workspaceName!, exact: true })
      .click();
    await copyDialog.getByRole("button", { name: "Copy", exact: true }).click();
    await expect(copyDialog).toBeHidden();

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await expect(testPage.getByText(copiedName, { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile secrets after copy/move");

    // Touch emulation does not move focus on tap, so focus restoration is
    // covered in the desktop spec (keyboard/mouse focus contract) instead.
    await prCapture.screenshot("mobile-secrets-copy-move", {
      caption: "Mobile secrets copy/move flow",
    });
  });

  test("keeps the dialog usable on a short viewport when an error is shown", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 480 });
    const sourceName = `E2E Short Viewport ${Date.now()}`;
    await apiClient.createSecret(sourceName, "short-viewport-value", {
      scope: "workspace",
      workspaceId: seedData.workspaceId,
    });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await testPage.getByRole("button", { name: `Copy or move ${sourceName}` }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    // Trigger an in-dialog error: an overlong target name (101 bytes).
    await dialog.getByLabel("Name").fill("x".repeat(101));
    await expect(dialog.getByText(/at most 100 bytes/)).toBeVisible();

    // The footer stays within the viewport and the sheet never exceeds it;
    // the middle section scrolls internally instead.
    const submitBox = await dialog.getByRole("button", { name: "Copy", exact: true }).boundingBox();
    expect(submitBox).not.toBeNull();
    expect(submitBox!.y + submitBox!.height).toBeLessThanOrEqual(480);
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(480);
    const documentOverflow = await testPage.evaluate(
      () => document.documentElement.scrollHeight > window.innerHeight,
    );
    expect(documentOverflow).toBe(false);
  });
});
