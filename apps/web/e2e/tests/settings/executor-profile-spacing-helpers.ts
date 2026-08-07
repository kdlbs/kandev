import { expect, type Page } from "@playwright/test";

const PROFILE_CARD_TITLES = ["Profile Details", "Environment Variables", "Prepare Script"] as const;

export async function expectExecutorProfileCardsSeparated(page: Page) {
  const profileFieldset = page.locator("fieldset").filter({ hasText: "Profile Details" });

  for (const title of PROFILE_CARD_TITLES) {
    await expect(profileFieldset.getByText(title, { exact: true })).toBeVisible();
  }

  const cards = profileFieldset.locator(':scope > [data-slot="card"]:visible');
  const cardBoxes = await cards.evaluateAll((elements) =>
    elements.map((element) => {
      const box = element.getBoundingClientRect();
      return { top: box.top, bottom: box.bottom };
    }),
  );

  expect(cardBoxes.length).toBeGreaterThanOrEqual(PROFILE_CARD_TITLES.length);

  for (let index = 1; index < cardBoxes.length; index += 1) {
    const gap = cardBoxes[index].top - cardBoxes[index - 1].bottom;
    expect(gap, `card ${index} should be separated from the previous card`).toBeGreaterThanOrEqual(
      31,
    );
  }
}

export async function expectNoHorizontalOverflow(page: Page) {
  const documentWidth = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));

  expect(documentWidth.scrollWidth).toBeLessThanOrEqual(documentWidth.clientWidth + 1);
}
