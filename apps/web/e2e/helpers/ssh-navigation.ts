import { expect, type Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";
import { dwell } from "./causal-waits";

const manager = "/settings/executors/ssh-connections";

export async function exerciseSSHNavigation(page: Page, touch: boolean) {
  await page.goto("/settings/executors");
  const entry = page.getByRole("link", { name: "SSH connections", exact: true });
  await expect(entry).toHaveCount(1);
  await expect(entry).toBeVisible();
  expect((await requireBox(entry, "SSH manager entry")).height).toBeGreaterThanOrEqual(
    touch ? 44 : 28,
  );
  const titleBox = await requireBox(
    page.getByRole("heading", { name: "Executors", exact: true }),
    "Executors title",
  );
  const entryBox = await requireBox(entry, "SSH header action");
  if ((page.viewportSize()?.width ?? 1280) >= 768)
    expect(Math.abs(entryBox.y - titleBox.y)).toBeLessThanOrEqual(1);
  else expect(entryBox.y).toBeGreaterThanOrEqual(titleBox.y + titleBox.height);
  await entry.click();
  await expect(page).toHaveURL(manager);
  await expect(page.getByRole("button", { name: "New connection", exact: true })).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
  await page.goBack();
  await expect(page).toHaveURL("/settings/executors");
  await page.goForward();
  await expect(page).toHaveURL(manager);
  await page.reload();
  await expect(page.getByRole("button", { name: "New connection", exact: true })).toBeVisible();
}

export async function exerciseSSHBookmark(page: Page) {
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/ssh/managed/") && request.method() !== "GET")
      mutations.push(request.url());
  });
  const query =
    "?tab=identities&new=1&adopt_executor=executor&adopt_digest=digest&adopt_connection=connection&return_connection=destination#identity";
  await page.goto("/settings/executors");
  await page.goto(`/settings/ssh-connections${query}`);
  await expect(page).toHaveURL(`${manager}${query}`);
  await expect(page.getByRole("button", { name: "Create identity", exact: true })).toBeVisible();
  await dwell(
    page,
    250,
    "negative-assertion",
    "redirect and identity form mount must not submit any SSH mutation",
  );
  expect(mutations).toEqual([]);
  await page.goBack();
  await expect(page).toHaveURL("/settings/executors");
}

export async function exerciseSSHSearch(page: Page, mobile: boolean) {
  await page.goto(mobile ? "/settings" : "/settings/executors");
  const scope = page.getByTestId(mobile ? "settings-index" : "app-sidebar-settings-mode");
  await scope.getByRole("searchbox", { name: "Search settings" }).fill("SSH connections");
  const result = scope.locator(`a[href="${manager}"]`);
  await expect(result).toHaveCount(1);
  await expect(result).toContainText("Executors");
  await result.click();
  await expect(page).toHaveURL(manager);
}

export async function exerciseSSHPseudo(page: Page) {
  await page.goto("/settings/executors");
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo");
  const link = page.locator(`main a[href="${manager}"]`);
  await expect(link).toBeVisible();
  await expect(link).not.toHaveText("SSH connections");
  await link.click();
  await expect(page).toHaveURL(manager);
  await assertNoDocumentHorizontalOverflow(page);
}
