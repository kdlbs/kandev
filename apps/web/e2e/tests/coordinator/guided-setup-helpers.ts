import { expect, type Locator, type Page } from "@playwright/test";

async function selectProfile(page: Page, trigger: Locator, profileId: string) {
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  const popupId = await trigger.getAttribute("aria-controls");
  expect(popupId).toBeTruthy();
  const popup = page.locator(`[id="${popupId}"]`);
  await popup.locator(`[role="option"][data-value="${profileId}"]`).click();
  // Wait for the closing popup and its focus scope to unmount before opening another picker.
  await expect(popup).toHaveCount(0);
}

export async function chooseIdentity(
  page: Page,
  name: string,
  profiles: { agentProfileId: string; worktreeExecutorProfileId: string },
) {
  await page.getByLabel("Name").fill(name);
  await selectProfile(
    page,
    page.getByTestId("coordinator-agent-profile-picker"),
    profiles.agentProfileId,
  );
  // The first executor belongs to the coordinator; the second belongs to created tasks.
  const executors = page.getByTestId("executor-profile-selector");
  await expect(executors).toHaveCount(2);
  await selectProfile(page, executors.nth(0), profiles.worktreeExecutorProfileId);
}
