import { expect, type Page } from "@playwright/test";

export async function disableRandomUuid(page: Page) {
  await page.addInitScript(() => {
    Object.defineProperty(window.crypto, "randomUUID", { configurable: true, value: undefined });
  });
}

export async function readSavedQuerySettings(page: Page, workspaceId: string) {
  const response = await page.request.get("/api/v1/github/workspace-settings", {
    params: { workspace_id: workspaceId },
  });
  expect(response.ok()).toBe(true);
  return response.json();
}

export async function restoreSavedQueries(page: Page, workspaceId: string, savedPresets: unknown) {
  const response = await page.request.put("/api/v1/github/workspace-settings", {
    data: { workspace_id: workspaceId, saved_presets: savedPresets ?? [] },
  });
  expect(response.ok(), "restore saved-query baseline").toBe(true);
}

export async function expectSavedQuery(page: Page, workspaceId: string, label: string) {
  const settings = await readSavedQuerySettings(page, workspaceId);
  const matches = settings.saved_presets.filter(
    (preset: { label: string }) => preset.label === label,
  );
  expect(matches).toHaveLength(1);
  expect(matches[0]).toMatchObject({ label, kind: "pr", customQuery: "is:open", repoFilter: "" });
  expect(matches[0].id).toMatch(
    /^p_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );
}
