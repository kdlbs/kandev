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
  test("opens the Add panel menu via touch without auto-adding the first missing panel", async ({
    testPage,
  }) => {
    // Regression guard: Radix's DropdownMenu opens on pointerdown and
    // tracks the pointer session through to release. A tap presses and
    // releases at the same coordinates with no movement, and used to
    // race the menu's first render - the pointerup landed on the
    // just-opened first item ("PR Details") and silently added it
    // before the user ever chose anything. See layout-editor-toolbar.tsx
    // AddPanelAction for the fix (deferred, click-driven open state).
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.openFromMobileMenu();
    const trigger = testPage.getByTestId("layout-editor-add-panel").getByRole("button", {
      name: "Add panel",
    });
    await trigger.tap();
    await expect(testPage.getByRole("menuitem", { name: "PR Details", exact: true })).toBeVisible();
    await expect(layouts.editor.locator(".dv-tab", { hasText: "PR Details" })).toHaveCount(0);
  });

  test("moves PR Details with touch controls without horizontal overflow", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.openFromMobileMenu();

    await expect(layouts.editor.locator(".dv-tab", { hasText: "PR Details" })).toHaveCount(0);
    await layouts.addPanel("PR Details", true);
    const prDetailsTab = layouts.editor.locator(".dv-tab", { hasText: "PR Details" });
    await expect(prDetailsTab).toBeVisible();
    await prDetailsTab.tap();
    await prCapture.screenshot("pr-details-touch-layout-editor", {
      caption: "Mobile layout editor selects PR Details through touch controls",
    });
    const moveLeft = layouts.actions.getByRole("button", { name: "Move tab left" });
    await expect(moveLeft).toBeEnabled();
    await moveLeft.tap();
    await layouts.save();

    const response = await apiClient.getUserSettings();
    const profile = (response.settings.saved_layouts as SavedProfile[]).find(
      (candidate) => candidate.is_default,
    );
    expect(profile, "Default saved layout is present").toBeDefined();
    if (!profile) return;
    const prDetailsGroup = profile.layout.columns
      .flatMap((column) => column.groups)
      .find((group) => group.panels.some((panel) => panel.id === "pr-detail"));
    expect(prDetailsGroup?.panels.map((panel) => panel.id)).toEqual(["pr-detail", "chat"]);

    await assertNoDocumentHorizontalOverflow(testPage, "PR Details mobile layout edit");
    await assertNoDescendantOverflowsRight(layouts.root, "PR Details mobile layout settings");
  });

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
