import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test("mobile navigation reaches Message Queue with touch-safe shared settings layout", async ({
  testPage,
}) => {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.click();
  const homeMenu = testPage.getByTestId("mobile-home-menu-card");
  await homeMenu.getByRole("link", { name: "Settings" }).click();
  await testPage.getByTestId("settings-mobile-menu-button").click();

  const menu = testPage.getByTestId("settings-mobile-menu");
  await menu.getByRole("button", { name: "Expand General" }).click();
  await menu.getByRole("link", { name: "Message Queue" }).click();

  await expect(testPage).toHaveURL(
    (url) => new URL(url).pathname === "/settings/general/message-queue",
  );
  await expect(testPage.getByTestId("system-page-title")).toHaveText("Message Queue");

  const input = testPage.getByTestId("message-queue-max-per-session");
  await expect(input).toBeVisible();
  const inputBox = await input.boundingBox();
  expect(inputBox).not.toBeNull();
  expect(inputBox!.height).toBeGreaterThanOrEqual(44);

  await expect
    .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true);
  await expect(testPage.getByTestId("settings-scroll-container")).toHaveCSS("overflow-y", "auto");
  const nestedScrollOwners = await testPage.getByTestId("message-queue-settings").evaluate(
    (root) =>
      Array.from(root.querySelectorAll("*")).filter((element) => {
        const overflow = getComputedStyle(element).overflowY;
        return overflow === "auto" || overflow === "scroll";
      }).length,
  );
  expect(nestedScrollOwners).toBe(0);

  const current = await input.inputValue();
  await input.fill(current === "23" ? "24" : "23");
  const saveBar = testPage.getByTestId("settings-floating-save");
  await expect(saveBar).toBeVisible();
  const [inputAfterEdit, saveBox] = await Promise.all([input.boundingBox(), saveBar.boundingBox()]);
  expect(inputAfterEdit).not.toBeNull();
  expect(saveBox).not.toBeNull();
  expect(inputAfterEdit!.y + inputAfterEdit!.height).toBeLessThan(saveBox!.y);
});
