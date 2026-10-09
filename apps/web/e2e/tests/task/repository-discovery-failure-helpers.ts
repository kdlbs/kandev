import type { Page } from "@playwright/test";

export const DISCOVERY_FAILURE_ROOT = "/srv/repositories/inaccessible-root";
export const DISCOVERED_REPOSITORY_PATH = "/srv/repositories/healthy-project";

type DiscoveryResponse = {
  roots: string[];
  repositories: Array<{ path: string; name: string; default_branch: string }>;
  total: number;
  desktop_runtime: boolean;
  root_states: [];
  scan_time: string;
  refreshing: boolean;
  cached: boolean;
  home_confirmation_required: boolean;
  failed_roots: string[];
};

function discoveryResponse(): DiscoveryResponse {
  const repositories = [
    {
      path: DISCOVERED_REPOSITORY_PATH,
      name: "healthy-project",
      default_branch: "main",
    },
  ];
  return {
    roots: ["/srv/repositories", DISCOVERY_FAILURE_ROOT],
    repositories,
    total: repositories.length,
    desktop_runtime: false,
    root_states: [],
    scan_time: "2026-09-17T10:00:00.000Z",
    refreshing: false,
    cached: true,
    home_confirmation_required: false,
    failed_roots: [DISCOVERY_FAILURE_ROOT],
  };
}

export async function installRepositoryDiscoveryFailureRoute(page: Page): Promise<void> {
  await page.route("**/api/v1/workspaces/*/repositories/discovery**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(discoveryResponse()),
    });
  });
}
