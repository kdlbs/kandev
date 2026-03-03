import { test, expect } from "../fixtures/test-base";

test.describe("Editor Provider Settings", () => {
  test("displays all four provider dropdowns", async ({ testPage }) => {
    await testPage.goto("/settings/general/editors");

    const section = testPage.getByTestId("editor-provider-section");
    await expect(section).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByTestId("editor-provider-card-code-editor")).toBeVisible();
    await expect(testPage.getByTestId("editor-provider-card-diff-viewer")).toBeVisible();
    await expect(testPage.getByTestId("editor-provider-card-chat-code-block")).toBeVisible();
    await expect(testPage.getByTestId("editor-provider-card-chat-diff")).toBeVisible();
  });

  test("shows correct default providers", async ({ testPage }) => {
    await testPage.goto("/settings/general/editors");
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });

    // Defaults per editor-resolver-store.ts
    await expect(testPage.getByTestId("editor-provider-select-code-editor")).toHaveText("Monaco");
    await expect(testPage.getByTestId("editor-provider-select-diff-viewer")).toHaveText(
      "Pierre Diffs",
    );
    await expect(testPage.getByTestId("editor-provider-select-chat-code-block")).toHaveText(
      "Shiki",
    );
    await expect(testPage.getByTestId("editor-provider-select-chat-diff")).toHaveText(
      "Pierre Diffs",
    );
  });

  test("switching a provider updates the select", async ({ testPage }) => {
    await testPage.goto("/settings/general/editors");
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });

    // Change diff viewer from Pierre Diffs to Monaco
    await testPage.getByTestId("editor-provider-select-diff-viewer").click();
    await testPage.getByRole("option", { name: "Monaco" }).click();

    await expect(testPage.getByTestId("editor-provider-select-diff-viewer")).toHaveText("Monaco");
  });

  test("provider change persists after page reload", async ({ testPage }) => {
    await testPage.goto("/settings/general/editors");
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });

    // Change code editor to CodeMirror
    await testPage.getByTestId("editor-provider-select-code-editor").click();
    await testPage.getByRole("option", { name: "CodeMirror" }).click();
    await expect(testPage.getByTestId("editor-provider-select-code-editor")).toHaveText(
      "CodeMirror",
    );

    // Reload and verify persistence
    await testPage.reload();
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("editor-provider-select-code-editor")).toHaveText(
      "CodeMirror",
    );
  });

  test("multiple provider changes persist independently", async ({ testPage }) => {
    await testPage.goto("/settings/general/editors");
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });

    // Change diff viewer to Monaco
    await testPage.getByTestId("editor-provider-select-diff-viewer").click();
    await testPage.getByRole("option", { name: "Monaco" }).click();

    // Change chat code blocks to CodeMirror
    await testPage.getByTestId("editor-provider-select-chat-code-block").click();
    await testPage.getByRole("option", { name: "CodeMirror" }).click();

    // Reload and verify both changes persisted while others remain default
    await testPage.reload();
    await expect(testPage.getByTestId("editor-provider-section")).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByTestId("editor-provider-select-code-editor")).toHaveText("Monaco");
    await expect(testPage.getByTestId("editor-provider-select-diff-viewer")).toHaveText("Monaco");
    await expect(testPage.getByTestId("editor-provider-select-chat-code-block")).toHaveText(
      "CodeMirror",
    );
    await expect(testPage.getByTestId("editor-provider-select-chat-diff")).toHaveText(
      "Pierre Diffs",
    );
  });
});
