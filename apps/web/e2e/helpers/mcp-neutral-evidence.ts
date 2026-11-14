import { expect, type Locator, type Page } from "@playwright/test";
import { SessionPage } from "../pages/session-page";
import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";
import pseudoTask from "../../src/locales/pseudo/task.json";

export async function expectNeutralMCPEvidence(row: Locator) {
  await expect(row.locator('span[aria-hidden="true"]').first()).toHaveClass(
    /bg-muted-foreground\/50/,
  );
}

// @covers AC-AGENTS-MCP-MANAGEMENT-003.2, AC-AGENTS-MCP-MANAGEMENT-003.3
export async function captureNeutralMCPEvidence(page: Page, mobile: boolean) {
  const capture = new PrAssetCapture(page, "mcp-neutral-evidence.spec.ts", {
    captureKey: mobile ? "phone" : "desktop",
  });
  if (!capture.capturing || (mobile && page.viewportSize()?.width !== 393)) return;
  const original = await page.evaluate(() => ({
    theme: localStorage.getItem("theme"),
    locale: document.cookie.match(/(?:^|; )kandev_locale=([^;]*)/)?.[1] ?? "en",
  }));
  const session = new SessionPage(page);
  const explorer = page.getByTestId("mcp-server-explorer");
  try {
    for (const [theme, locale] of [
      ["light", "en"],
      ["dark", "en"],
      ["dark", "pseudo"],
    ]) {
      await page.evaluate(
        ({ theme, locale }) => {
          localStorage.setItem("theme", theme);
          document.cookie = `kandev_locale=${locale}; path=/; max-age=31536000; SameSite=Lax`;
        },
        { theme, locale },
      );
      await page.reload();
      await session.waitForLoad();
      await page.getByTestId("mcp-status-trigger").click();
      await explorer.getByTestId("mcp-connections-tab").click();
      await expect(page.locator("html")).toHaveClass(new RegExp(theme));
      await expect(explorer.getByTestId("mcp-server-row-kandev")).toContainText(
        locale === "pseudo" ? pseudoTask.mcpStatusDelivered : "Delivered, connection unverified",
      );
      await expectNeutralMCPEvidence(explorer.getByTestId("mcp-server-row-kandev"));
      await waitForFiniteAnimations(explorer);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await capture.screenshot(`${theme}-${locale}`, {
        caption: `${mobile ? "Phone" : "Desktop"} neutral MCP delivery beside saved changes (${theme}, ${locale})`,
      });
    }
  } finally {
    capture.flush();
    await page.evaluate(({ theme, locale }) => {
      if (theme === null) localStorage.removeItem("theme");
      else localStorage.setItem("theme", theme);
      document.cookie = `kandev_locale=${locale}; path=/; max-age=31536000; SameSite=Lax`;
    }, original);
    await page.reload();
    await session.waitForLoad();
    await page.getByTestId("mcp-status-trigger").click();
    await explorer.getByTestId("mcp-connections-tab").click();
  }
}
