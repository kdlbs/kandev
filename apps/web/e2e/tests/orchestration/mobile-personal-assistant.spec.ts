import { test, expect } from "../../fixtures/test-base";
import { ASSISTANT_ENV, exerciseExampleAssistant } from "../../helpers/personal-assistant";
test("phone assistant navigation, native answer and memory controls fit one column", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180000);
  await backend.restart(ASSISTANT_ENV);
  await page.goto("/");
  page.setDefaultTimeout(15000);
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  await page.getByTestId("mobile-home-menu-card").getByTestId("assistant-nav").click();
  await expect(page).toHaveURL(/\/assistant$/);
  await exerciseExampleAssistant(page, backend, apiClient, seedData, {
    mobile: true,
    capture: prCapture,
  });
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo");
  await expect(page.locator("#assistant-tab-chat")).toContainText(/[^\u0000-\u007f]/);
  await page.locator("#assistant-tab-details").click();
  await expect(page.getByText("Prepare the example guide", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const target = await page.locator("#assistant-tab-attention").boundingBox();
  expect(target?.height).toBeGreaterThanOrEqual(44);
  await page.screenshot({ path: test.info().outputPath("assistant-phone-pseudo.png") });
});
