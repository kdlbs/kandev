import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

type WorkspaceTabSettingsHomeOptions = {
  page: Page;
  apiClient: ApiClient;
  backendPort: number;
  activeWorkspaceId: string;
  mobile: boolean;
};

/** Exercise Settings-to-Home navigation while another tab owns the shared cookie. */
export async function expectSettingsHomeToKeepTabWorkspace({
  page,
  apiClient,
  backendPort,
  activeWorkspaceId,
  mobile,
}: WorkspaceTabSettingsHomeOptions): Promise<void> {
  await page.goto("/");
  await expect(workspacePickerTrigger(page, mobile)).toBeVisible();

  const workspaceB = await apiClient.createWorkspace(`Workspace tab B ${Date.now()}`);
  const pageB = await page.context().newPage();
  try {
    await pageB.addInitScript((port: string) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = port;
      window.__KANDEV_E2E_EXPOSE_STORE__ = true;
    }, String(backendPort));
    await pageB.goto("/");
    await expect(workspacePickerTrigger(pageB, mobile)).toBeVisible();
    await selectWorkspace(pageB, workspaceB.id, mobile);
    await expectHomeWorkspace(pageB, workspaceB.id);

    await openSettings(page, mobile);
    await returnHome(page, activeWorkspaceId, mobile);

    await openSettings(pageB, mobile);
    await returnHome(pageB, workspaceB.id, mobile);
  } finally {
    await pageB.close();
    await apiClient.deleteWorkspace(workspaceB.id, workspaceB.name);
  }
}

function workspacePickerTrigger(page: Page, mobile: boolean) {
  return page.getByTestId(mobile ? "app-nav-trigger" : "sidebar-workspace-trigger");
}

async function selectWorkspace(page: Page, workspaceId: string, mobile: boolean): Promise<void> {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").click();
    await page
      .getByRole("dialog", { name: "Menu" })
      .getByTestId("mobile-workspace-trigger")
      .click();
    await page.getByTestId(`mobile-workspace-item-${workspaceId}`).click();
  } else {
    await page.getByTestId("sidebar-workspace-trigger").click();
    await page.getByTestId(`sidebar-workspace-item-${workspaceId}`).click();
  }
}

async function openSettings(page: Page, mobile: boolean): Promise<void> {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").click();
    await page
      .getByRole("dialog", { name: "Menu" })
      .getByRole("link", { name: "Settings", exact: true })
      .click();
  } else {
    await page.getByTestId("sidebar-settings-gear").click();
  }
  await expect(page).toHaveURL((url) => url.pathname.startsWith("/settings"));
}

async function returnHome(page: Page, workspaceId: string, mobile: boolean): Promise<void> {
  const homeLink = mobile
    ? page.getByTestId("topbar-phone-home").getByRole("link")
    : page.getByTestId("app-sidebar-header").getByRole("link");
  await expect(homeLink).toBeVisible();
  await homeLink.click();
  await expectHomeWorkspace(page, workspaceId);
}

async function expectHomeWorkspace(page: Page, workspaceId: string): Promise<void> {
  await expect(page).toHaveURL(
    (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === workspaceId,
  );
}
