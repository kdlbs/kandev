// REQ-COORDINATOR-AGENT-NOTES on a phone: one column, a full-screen note
// detail with a Back control, 44 px touch targets and no horizontal overflow.
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";
import { NOTES_ENV, NOTE_TEXT, setupNotes } from "./agent-notes-fixture";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator Notes section on a phone viewport", () => {
  test("targets are 44 px, detail is full screen and nothing overflows", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(NOTES_ENV);
    try {
      const { coordinator, taskId } = await setupNotes(apiClient, seedData, "Mobile Notes");
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=notes`,
      );
      const add = testPage.getByTestId("note-add");
      await expect(add).toBeVisible();
      expect((await add.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      await add.tap();
      await expect(testPage.getByTestId("notes-back")).toBeVisible();
      await testPage.getByTestId("note-text").fill(NOTE_TEXT);
      await testPage.getByLabel("Identifier").fill(taskId);
      const save = testPage.getByTestId("note-save");
      expect((await save.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      await save.tap();
      await expect(testPage.getByTestId("note-detail")).toContainText(NOTE_TEXT);
      await expect(testPage.getByTestId("note-list")).toBeHidden();
      const fits = await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      );
      expect(fits).toBe(true);
      await testPage.getByTestId("notes-back").tap();
      await expect(testPage.locator('[data-testid^="note-row-"]').first()).toBeVisible();
    } finally {
      await release();
    }
  });
});
