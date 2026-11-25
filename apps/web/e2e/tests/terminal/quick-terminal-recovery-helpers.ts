import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { readTerminalHostBuffer } from "./terminal-test-helpers";

export type QuickTerminalDescriptor = {
  tabId: string;
  workspaceId: string;
  sessionId: string | null;
  sequence: number;
  status: "connecting" | "running" | "exited" | "error";
  exitCode?: number;
  error?: string;
};

export async function listQuickTerminalDescriptors(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<QuickTerminalDescriptor[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/quick-terminal-tabs?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!response.ok)
    throw new Error(`Unable to read Quick Terminal descriptors (${response.status})`);
  const result = (await response.json()) as { tabs: QuickTerminalDescriptor[] };
  return result.tabs;
}

export async function readQuickTerminalDescriptor(
  apiClient: ApiClient,
  workspaceId: string,
  tabId: string,
): Promise<QuickTerminalDescriptor> {
  const descriptor = (await listQuickTerminalDescriptors(apiClient, workspaceId)).find(
    (tab) => tab.tabId === tabId,
  );
  if (!descriptor) throw new Error(`Quick Terminal descriptor ${tabId} was not found`);
  return descriptor;
}

export async function waitForTerminalText(
  terminal: Locator,
  expected: string,
  normalize = false,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const output = await readTerminalHostBuffer(terminal);
        return normalize ? output.replace(/\s+/g, "") : output;
      },
      { timeout: 20_000, message: `Waiting for Quick Terminal output ${expected}` },
    )
    .toContain(normalize ? expected.replace(/\s+/g, "") : expected);
}

export async function openDesktopQuickTerminal(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.getByTestId("sidebar-quick-terminal-shortcut").click();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
  return dialog;
}

export async function openMobileQuickTerminal(page: Page): Promise<Locator> {
  await page.goto("/");
  const menuButton = page.getByTestId("app-nav-trigger");
  if ((await menuButton.getAttribute("aria-expanded")) !== "true") await menuButton.tap();
  await expect(menuButton).toHaveAttribute("aria-expanded", "true");
  await page.getByTestId("mobile-quick-terminal-button").tap();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
  return dialog;
}

export async function closeQuickTerminals(page: Page, mobile = false): Promise<void> {
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  if (!(await dialog.isVisible().catch(() => false))) {
    if (mobile) {
      const menuButton = page.getByTestId("app-nav-trigger");
      if (!(await menuButton.isVisible().catch(() => false))) return;
      if ((await menuButton.getAttribute("aria-expanded")) !== "true") await menuButton.tap();
      await page.getByTestId("mobile-quick-terminal-button").tap();
    } else {
      const launcher = page.getByTestId("sidebar-quick-terminal-shortcut");
      if (!(await launcher.isVisible().catch(() => false))) return;
      await launcher.click();
    }
  }
  if (!(await dialog.isVisible().catch(() => false))) return;

  const tabs = dialog.locator('[data-testid="quick-terminal-tab"]');
  while ((await tabs.count()) > 0) {
    const count = await tabs.count();
    const tab = tabs.nth(count - 1);
    const sequence = await tab.getAttribute("data-terminal-sequence");
    if (!sequence) throw new Error("Quick Terminal tab has no sequence");
    const close = tab.getByRole("button", { name: `Close Terminal ${sequence}`, exact: true });
    if (await close.isVisible().catch(() => false)) {
      await close.click();
    } else {
      await tab.getByRole("button", { name: `Actions for Terminal ${sequence}` }).tap();
      await page.getByRole("menuitem", { name: `Close Terminal ${sequence}`, exact: true }).tap();
    }
    await expect(tabs).toHaveCount(count - 1);
  }
}
