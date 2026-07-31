import type { Page } from "@playwright/test";

const RATE_LIMIT_RESET_AT = "2030-01-01T00:00:00Z";

export async function stubGitHubRateLimits(page: Page, workspaceId: string) {
  await page.route("**/api/v1/github/status?*", async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("workspace_id") !== workspaceId) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const body = (await response.json()) as Record<string, unknown>;
    await route.fulfill({
      response,
      json: {
        ...body,
        rate_limit: {
          core: {
            resource: "core",
            remaining: 4321,
            limit: 5000,
            reset_at: RATE_LIMIT_RESET_AT,
            updated_at: "2029-12-31T23:45:00Z",
          },
          graphql: {
            resource: "graphql",
            remaining: 4900,
            limit: 5000,
            reset_at: RATE_LIMIT_RESET_AT,
            updated_at: "2029-12-31T23:45:00Z",
          },
          search: {
            resource: "search",
            remaining: 25,
            limit: 30,
            reset_at: RATE_LIMIT_RESET_AT,
            updated_at: "2029-12-31T23:45:00Z",
          },
        },
      },
    });
  });
}
