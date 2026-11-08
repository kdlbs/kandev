// REQ-COORDINATOR-AGENT-NOTES: managers add, edit, revert and delete cited notes
// from the Notes section; a rejected save keeps the draft; the section is
// keyboard operable in English and in a non-Latin locale.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";
import { NOTES_ENV, NOTE_TEXT, setupNotes } from "./agent-notes-fixture";

test.describe("Coordinator Notes section", () => {
  test("adds, edits, reverts and deletes a note by keyboard", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const release = await backend.useEnv(NOTES_ENV);
    try {
      const { coordinator, taskId } = await setupNotes(apiClient, seedData, "Notes Coordinator");
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=notes`,
      );
      await expect(testPage.getByTestId("notes-empty")).toBeVisible();

      await testPage.getByTestId("note-add").focus();
      await testPage.keyboard.press("Enter");
      await testPage.getByTestId("note-text").fill(NOTE_TEXT);
      await testPage.getByLabel("Identifier").fill(taskId);
      const created = waitForHttp(testPage, "POST", /\/notes$/);
      await testPage.getByTestId("note-save").focus();
      await testPage.keyboard.press("Enter");
      expect((await created).status()).toBe(201);
      await expect(testPage.getByTestId("note-detail")).toContainText(NOTE_TEXT);

      await testPage.getByTestId("note-edit").click();
      await testPage.getByTestId("note-text").fill("Releases ship on Wednesdays");
      const edited = waitForHttp(testPage, "PUT", /\/notes\/[^/]+$/);
      await testPage.getByTestId("note-save").click();
      expect((await edited).status()).toBe(200);
      await expect(testPage.getByTestId("note-version-2")).toBeVisible();

      const reverted = waitForHttp(testPage, "POST", /\/notes\/[^/]+\/revert$/);
      await testPage.getByTestId("note-revert-1").click();
      expect((await reverted).status()).toBe(200);
      await expect(testPage.getByTestId("note-version-3")).toBeVisible();

      await testPage.getByTestId("note-delete").click();
      const deleted = waitForHttp(testPage, "POST", /\/notes\/[^/]+\/delete$/);
      await testPage.getByTestId("note-delete-confirm").click();
      expect((await deleted).status()).toBe(200);
      await expect(testPage.getByTestId("notes-empty")).toBeVisible();
      await testPage.getByTestId("notes-include-deleted").click();
      await expect(testPage.getByTestId("note-list")).toContainText("Deleted");
    } finally {
      await release();
    }
  });

  test("keeps the draft when a citation cannot be resolved", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(NOTES_ENV);
    try {
      const { coordinator } = await setupNotes(apiClient, seedData, "Rejected Notes");
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=notes`,
      );
      await testPage.getByTestId("note-add").click();
      await testPage.getByTestId("note-text").fill(NOTE_TEXT);
      await testPage.getByLabel("Identifier").fill("00000000-0000-4000-8000-000000000000");
      await testPage.getByTestId("note-save").click();
      await expect(testPage.getByTestId("note-editor-error")).toBeVisible();
      await expect(testPage.getByTestId("note-text")).toHaveValue(NOTE_TEXT);
    } finally {
      await release();
    }
  });

  test("renders in a non-Latin locale", async ({ testPage, backend, apiClient, seedData }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(NOTES_ENV);
    try {
      const { coordinator } = await setupNotes(apiClient, seedData, "Japanese Notes");
      await testPage
        .context()
        .addCookies([{ name: "kandev_locale", value: "ja", url: backend.frontendUrl }]);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=notes`,
      );
      await expect(testPage.getByTestId("notes-empty")).toHaveText("ノートはまだありません。");
      await expect(testPage.getByTestId("note-add")).toHaveText("ノートを追加");
    } finally {
      await release();
    }
  });
});
