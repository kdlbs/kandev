import type { Page } from "@playwright/test";

const RATE_LIMIT_RESET_AT = "2030-01-01T00:00:00Z";

const REFRESHED_RATE_LIMIT = {
  core: {
    resource: "core",
    remaining: 3210,
    limit: 5000,
    reset_at: RATE_LIMIT_RESET_AT,
    updated_at: "2029-12-31T23:59:00Z",
  },
  graphql: {
    resource: "graphql",
    remaining: 4789,
    limit: 5000,
    reset_at: RATE_LIMIT_RESET_AT,
    updated_at: "2029-12-31T23:59:00Z",
  },
  search: {
    resource: "search",
    remaining: 24,
    limit: 30,
    reset_at: RATE_LIMIT_RESET_AT,
    updated_at: "2029-12-31T23:59:00Z",
  },
};

export async function stubGitHubRateLimits(page: Page, workspaceId: string) {
  await page.route("**/api/v1/github/status?*", async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("workspace_id") !== workspaceId) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const body = (await response.json()) as Record<string, unknown>;
    const refreshed = url.searchParams.get("refresh_rate_limit") === "true";
    if (refreshed) {
      body.rate_limit = REFRESHED_RATE_LIMIT;
    } else {
      delete body.rate_limit;
    }
    await route.fulfill({
      response,
      json: body,
    });
  });
}
