import { expect, type Page } from "@playwright/test";

export async function repeatTokenCreation(page: Page, touch: boolean) {
  const activate = async (testId: string) => {
    const target = page.getByTestId(testId);
    if (touch) await target.tap();
    else await target.click();
  };
  await page.goto("/settings/account/tokens");
  await expect(page.getByTestId("api-tokens-card")).toBeVisible();
  await activate("api-tokens-create");
  await page.getByTestId("api-tokens-name").fill("first fixture token");
  await activate("api-tokens-mint-submit");
  const firstField = page.getByTestId("api-tokens-raw-value");
  await expect(firstField).toBeVisible();
  const first = await firstField.inputValue();
  const done = page.getByRole("button", { name: "Done" });
  if (touch) await done.tap();
  else await done.click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await activate("api-tokens-create");
  await expect(page.getByTestId("api-tokens-name")).toHaveValue("");
  await expect(page.getByTestId("api-tokens-raw-value")).toHaveCount(0);
  await page.getByTestId("api-tokens-name").fill("second fixture token");
  await activate("api-tokens-mint-submit");
  const secondField = page.getByTestId("api-tokens-raw-value");
  await expect(secondField).toBeVisible();
  const second = await secondField.inputValue();
  expect(first.length > 0 && second.length > 0 && first !== second).toBe(true);
  const finish = page.getByRole("button", { name: "Done" });
  if (touch) await finish.tap();
  else await finish.click();
  await activate("api-tokens-create");
  await page.getByTestId("api-tokens-name").fill("discard outside");
  await page.locator('[data-slot="dialog-overlay"]').click({ position: { x: 2, y: 2 } });
  await expect(page.getByRole("dialog")).toBeHidden();
  await activate("api-tokens-create");
  await expect(page.getByTestId("api-tokens-name")).toHaveValue("");
  if (touch) {
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  }
}
