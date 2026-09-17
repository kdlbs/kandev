import type { Page } from "@playwright/test";

export const DISCOVERY_FAILURE_ROOTS = [
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/January/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/February/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/March/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/April/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/May/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/June/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/July/Private Archive",
  "/srv/repositories/Photo Booth Library/Protected/Very Long Folder Name/Camera Imports/2026/August/Private Archive",
] as const;
export const DISCOVERY_FAILURE_ROOT = DISCOVERY_FAILURE_ROOTS[0];
export const DISCOVERED_REPOSITORY_PATH = "/srv/repositories/healthy-project";
export const RECOVERED_REPOSITORY_PATH = "/srv/repositories/recovered-project";

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

function discoveryResponse(recovered: boolean): DiscoveryResponse {
  const repositories = [
    {
      path: DISCOVERED_REPOSITORY_PATH,
      name: "healthy-project",
      default_branch: "main",
    },
  ];
  if (recovered) {
    repositories.push({
      path: RECOVERED_REPOSITORY_PATH,
      name: "recovered-project",
      default_branch: "main",
    });
  }
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
    failed_roots: recovered ? [] : [...DISCOVERY_FAILURE_ROOTS],
  };
}

export async function installRepositoryDiscoveryFailureRoute(page: Page): Promise<void> {
  let recovered = false;
  await page.route("**/api/v1/workspaces/*/repositories/discovery**", async (route) => {
    if (route.request().method() === "POST") recovered = true;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(discoveryResponse(recovered)),
    });
  });
}
