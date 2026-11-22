import { expect, test, type Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";
import { dwell } from "./causal-waits";

const manager = "/settings/executors/ssh-connections";

export async function exerciseSSHNavigation(page: Page, touch: boolean) {
  await page.goto("/settings/executors");
  const entry = page.locator("main").getByRole("link", { name: "SSH connections", exact: true });
  await expect(entry).toHaveCount(1);
  await expect(entry).toBeVisible();
  const title = page.getByRole("heading", { name: "Executors", exact: true });
  const titleBox = await requireBox(title, "Executors title");
  const entryBox = await requireBox(entry, "SSH navigation card");
  expect(entryBox.height).toBeGreaterThanOrEqual(44);
  expect(entryBox.y).toBeGreaterThan(titleBox.y + titleBox.height);
  expect(Math.abs(entryBox.x - titleBox.x)).toBeLessThanOrEqual(1);
  const contentBox = await requireBox(title.locator("../.."), "Executors header");
  expect(Math.abs(entryBox.width - contentBox.width)).toBeLessThanOrEqual(1);
  const profiles = await requireBox(
    page.getByRole("heading", { name: "Profiles", exact: true }),
    "Profiles heading",
  );
  expect(entryBox.y + entryBox.height).toBeLessThan(profiles.y);
  await page.screenshot({
    path: test.info().outputPath("ssh-executors-entry.png"),
    fullPage: true,
  });
  if (touch && (await page.evaluate(() => navigator.maxTouchPoints > 0))) await entry.tap();
  else await entry.click();
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

export async function exerciseSSHSidebar(page: Page, mobile: boolean) {
  await page.goto(mobile ? "/settings" : "/settings/executors/ssh-connections");
  const scope = page.getByTestId(mobile ? "settings-index" : "app-sidebar-settings-mode");
  const entry = scope.locator(`a[href="${manager}"]`);
  if (mobile) await scope.getByRole("button", { name: "Expand Executors", exact: true }).click();
  await expect(entry).toHaveCount(1);
  await expect(entry).toBeVisible();
  await expect(entry.locator("button")).toHaveCount(0);
  if (mobile) expect((await entry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const links = scope.locator('a[href^="/settings/executors"]');
  expect(await links.nth(1).getAttribute("href")).toBe(manager);
  if (mobile && (await page.evaluate(() => navigator.maxTouchPoints > 0))) await entry.tap();
  else await entry.press("Enter");
  await expect(page).toHaveURL(manager);
  if (!mobile) await expect(entry).toHaveAttribute("data-active", "true");
  await assertNoDocumentHorizontalOverflow(page);
}
