import { test, expect } from "../../fixtures/test-base";
import {
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";

type SavedProfile = {
  id: string;
  name: string;
  is_default: boolean;
  layout: {
    columns: Array<{
      groups: Array<{ panels: Array<{ id: string }> }>;
    }>;
  };
};

test.describe("Mobile layout profiles", () => {
  test("edits and reorders a profile without horizontal overflow", async ({
    testPage,
    apiClient,
  }) => {
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.openFromMobileMenu();
    await assertNoDocumentHorizontalOverflow(testPage, "layouts page");
    await assertNoDescendantOverflowsRight(layouts.root, "layouts settings");

    await layouts.selectPanel("Files");
    await layouts.moveSelectedTabRight();
    await layouts.save();

    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.saved_layouts, {
        timeout: 10_000,
        message: "Waiting for the reordered layout profile to persist",
      })
      .toHaveLength(1);
    const response = await apiClient.getUserSettings();
    const profile = (response.settings.saved_layouts as SavedProfile[])[0];
    expect(profile).toMatchObject({ id: "layout-override-default", name: "Default" });
    expect(profile).toMatchObject({ is_default: true });
    const filesGroup = profile.layout.columns
      .flatMap((column) => column.groups)
      .find((group) => group.panels.some((panel) => panel.id === "files"));
    expect(filesGroup?.panels.map((panel) => panel.id)).toEqual(["changes", "files"]);

    await assertNoDocumentHorizontalOverflow(testPage, "edited layouts page");
    await assertNoDescendantOverflowsRight(layouts.root, "edited layouts settings");
  });
});
