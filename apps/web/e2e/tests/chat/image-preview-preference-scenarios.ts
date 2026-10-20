import { test, expect } from "../../fixtures/test-base";
import { expectTouchTarget } from "./rich-output-image-helpers";
import { RICH_OUTPUT_IMAGE, seedRichOutputImage, seedRichOutputTask } from "./rich-output-helpers";

export function imagePreviewPreferenceScenario(mobile: boolean) {
  // @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.6
  test("image expansion preference resets, saves, and controls file previews", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.goto("/settings/preferences/appearance");
    const toggle = testPage.getByRole("switch", { name: "Expand image previews", exact: true });
    const save = testPage.getByTestId("settings-floating-save");
    const activate = async (control: typeof toggle) => {
      if (mobile) await control.tap();
      else await control.click();
    };
    await expect(toggle).toBeChecked();
    await toggle.scrollIntoViewIfNeeded();
    if (mobile) await expectTouchTarget(toggle);
    await activate(toggle);
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.settings.imagePreviewExpansion")),
    ).toBeNull();
    await activate(save.getByRole("button", { name: "Reset", exact: true }));
    await expect(toggle).toBeChecked();
    await activate(toggle);
    await activate(save.getByRole("button", { name: "Save changes", exact: true }));
    await expect(save).toBeHidden();
    await testPage.reload();
    await expect(toggle).not.toBeChecked();
    await testPage
      .getByTestId("image-preview-expansion-settings")
      .evaluate((row) => row.scrollIntoView({ block: "center" }));
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await testPage.screenshot({ path: test.info().outputPath("image-expansion-setting.png") });

    const session = await seedRichOutputTask({
      page: testPage,
      apiClient,
      seedData,
      title: "Image expansion preference",
      payload: seedRichOutputImage(seedData.repositoryPath),
    });
    const sessionUrl = testPage.url();
    const card = session
      .activeChat()
      .getByTestId("rich-output-file")
      .filter({ hasText: RICH_OUTPUT_IMAGE });
    const image = card.getByRole("img", { name: RICH_OUTPUT_IMAGE, exact: true });
    await expect(image).toBeVisible();
    await expect(
      card.getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true }),
    ).toHaveCount(0);
    await activate(image);
    await expect(testPage.getByRole("dialog", { name: "Image preview" })).toHaveCount(0);
    await expect(card.getByTestId("rich-output-file-preview-toggle")).toHaveAttribute(
      "aria-expanded",
      "true",
    );

    await testPage.goto("/settings/preferences/appearance");
    await activate(toggle);
    await activate(save.getByRole("button", { name: "Save changes", exact: true }));
    await expect(save).toBeHidden();
    await testPage.goto(sessionUrl);
    await session.waitForLoad();
    await activate(card.getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true }));
    await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeVisible();
  });
}
