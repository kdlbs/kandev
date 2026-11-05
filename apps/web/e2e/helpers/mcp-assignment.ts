import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";
import { expect, type Page, type Request, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import {
  assertNoDocumentHorizontalOverflow,
  assertLocatorWithinViewportX,
} from "./layout-assertions";

export const ASSIGNMENT_PREFIX = "e2e-assignment-";
type Source = "custom" | "marketplace";

async function openRegistration(page: Page, workspaceId: string, source: Source, runtime: string) {
  await page.goto(`/settings/workspaces/${workspaceId}/mcp-servers`);
  if (source === "custom") {
    await page.getByRole("button", { name: "Add MCP server" }).first().click();
    const form = page.getByTestId("mcp-definition-form");
    await form.getByLabel("Runtime name").fill(runtime);
    await form.getByLabel("Display name").fill("Documentation tools for the engineering workspace");
    await form.getByLabel("Setup mode").click();
    await page.getByRole("option", { name: "Remote endpoint" }).click();
    await form.getByLabel("Remote URL").fill("https://mcp.example.test/docs");
    return page.getByTestId("settings-floating-save").getByRole("button", { name: "Save changes" });
  }
  await page.getByRole("tab", { name: "Marketplace" }).click();
  const view = page.getByTestId("mcp-marketplace");
  const search = view.getByRole("textbox", { name: "Search MCP marketplace" });
  await search.fill("example");
  await search.press("Enter");
  await view
    .locator('[data-slot="card"]')
    .filter({ hasText: "Example tools" })
    .getByRole("button", { name: "Review" })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Runtime name").fill(runtime);
  return dialog.getByRole("button", { name: "Add MCP server" });
}

function captureRegistrationRequests(page: Page) {
  const writes: string[] = [];
  const endpointRequests: string[] = [];
  const listener = (request: Request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/") && !["GET", "HEAD", "OPTIONS"].includes(request.method()))
      writes.push(`${request.method()} ${url.pathname}`);
    if (url.hostname === "mcp.example.test") endpointRequests.push(request.url());
  };
  page.on("request", listener);
  return { writes, endpointRequests, stop: () => page.off("request", listener) };
}

function assertStorageOnly(
  writes: string[],
  endpointRequests: string[],
  workspaceId: string,
  source: Source,
) {
  const expected = `POST /api/v1/workspaces/${workspaceId}/${source === "custom" ? "mcp-servers" : "mcp-marketplace/install"}`;
  // Sidebar search uses POST for a read-only query when the page reloads.
  const sidebarQuery = `POST /api/v1/workspaces/${workspaceId}/sidebar/query`;
  expect(writes.filter((request) => request !== sidebarQuery)).toEqual([expected]);
  expect(endpointRequests).toEqual([]);
}

export async function checkSavedRegistration(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  { source, phone, info }: { source: Source; phone: boolean; info: TestInfo },
) {
  const assets = new PrAssetCapture(
    page,
    phone ? "mobile-mcp-assignment.spec.ts" : "mcp-assignment.spec.ts",
    { captureKey: `saved-${source}` },
  );
  const runtime = `${ASSIGNMENT_PREFIX}${source}-${phone ? "phone" : "desktop"}`;
  const before = await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId);
  const save = await openRegistration(page, seed.workspaceId, source, runtime);
  const capture = captureRegistrationRequests(page);
  try {
    await save.click();
    const receipt = page.getByTestId("mcp-registration-completion");
    await expect(receipt).toContainText("Saved to");
    await expect(page.getByTestId("mcp-definition-form")).toHaveCount(0);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Configured" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const matches = (await api.listMCPServers(seed.workspaceId)).filter(
      (server) => server.runtime_name === runtime,
    );
    expect(matches).toHaveLength(1);
    const created = matches[0];
    await expect(receipt).toContainText(created.display_name);
    await expect(page.getByTestId(`mcp-server-card-${created.id}`)).toHaveCount(1);
    const done = receipt.getByRole("button", { name: "Done", exact: true });
    if (phone) {
      expect((await done.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await assertLocatorWithinViewportX(receipt, "saved registration");
      await assertNoDocumentHorizontalOverflow(page, "saved registration");
      await page.screenshot({ path: info.outputPath(`saved-${source}-phone.png`), fullPage: true });
    }
    if (assets.capturing) {
      await waitForFiniteAnimations(receipt);
      await assets.screenshot("saved", {
        caption: "Saved MCP definition with optional Use in and Done",
      });
    }
    await receipt.getByRole("button", { name: "Use in", exact: true }).click();
    const chooser = page.getByTestId("mcp-assignment-chooser");
    await expect(chooser).toBeVisible();
    await chooser.getByRole("button", { name: "Close", exact: true }).first().click();
    await expect(receipt).toBeVisible();
    await done.click();
    await expect(receipt).toHaveCount(0);
    await page.reload();
    await expect(page.getByTestId(`mcp-server-card-${created.id}`)).toBeVisible();
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).toEqual(before.definition_ids);
    assertStorageOnly(capture.writes, capture.endpointRequests, seed.workspaceId, source);
  } finally {
    capture.stop();
    assets.flush();
  }
}

export async function checkUncertainRegistration(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  phone: boolean,
) {
  const runtime = `${ASSIGNMENT_PREFIX}uncertain-${phone ? "phone" : "desktop"}`;
  const save = await openRegistration(page, seed.workspaceId, "custom", runtime);
  const endpoint = `**/api/v1/workspaces/${seed.workspaceId}/mcp-servers`;
  await page.route(endpoint, async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    const response = await route.fetch();
    expect(response.ok()).toBe(true);
    await route.abort("failed");
  });
  const capture = captureRegistrationRequests(page);
  try {
    await save.click();
    const notice = page.getByTestId("mcp-registration-uncertain");
    await expect(notice).toContainText("could not confirm");
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
    await expect(page.getByTestId("mcp-registration-completion")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Add MCP server" }).first()).toBeDisabled();
    await notice.getByRole("button", { name: "Review saved servers" }).click();
    await expect(notice).toHaveCount(0);
    const matches = (await api.listMCPServers(seed.workspaceId)).filter(
      (server) => server.runtime_name === runtime,
    );
    expect(matches).toHaveLength(1);
    await expect(page.getByTestId(`mcp-server-card-${matches[0].id}`)).toBeVisible();
    await expect(page.getByTestId("mcp-registration-completion")).toHaveCount(0);
    assertStorageOnly(capture.writes, capture.endpointRequests, seed.workspaceId, "custom");
  } finally {
    capture.stop();
    await page.unroute(endpoint);
  }
}

export async function cleanupAssignmentServers(api: ApiClient, workspaceId: string) {
  for (const server of await api.listMCPServers(workspaceId)) {
    if (server.runtime_name.startsWith(ASSIGNMENT_PREFIX))
      await api.deleteMCPServer(workspaceId, server.id, server.revision);
  }
}
