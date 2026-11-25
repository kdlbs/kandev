import http from "node:http";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export async function fileViewerMarketplaceFlow(page: Page, api: ApiClient) {
  const plugins = [
    { id: "kandev-plugin-stl-filter", name: "STL filter fixture", file_extensions: ["stl"] },
    { id: "kandev-plugin-obj-filter", name: "OBJ filter fixture", file_extensions: ["obj"] },
  ].map((entry) => ({
    ...entry,
    version: "1.0.0",
    description: "File viewer filter fixture",
    author: "Kandev",
    categories: ["File viewers"],
    package_url: "https://example.com/plugin.tar.gz",
    repo_url: "https://example.com/repository",
    stars: 0,
  }));
  // Only the external registry is a fixture. The Kandev catalog and filtering are real.
  const server = http.createServer((_request, response) => {
    response.setHeader("Content-Type", "application/json");
    response.end(
      JSON.stringify({
        schema_version: 1,
        generated_at: "2026-11-18T00:00:00Z",
        source: { name: "Viewer filter fixture", url: "" },
        plugins,
      }),
    );
  });
  let sourceId = "";
  try {
    await new Promise<void>((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, "127.0.0.1", resolve);
    });
    const address = server.address();
    if (!address || typeof address === "string") throw Error("fixture address unavailable");
    const source = await api.rawRequest("POST", "/api/plugins/marketplace/sources", {
      name: "Viewer filter fixture",
      url: `http://127.0.0.1:${address.port}/index.json`,
    });
    expect(source.ok).toBe(true);
    sourceId = (await source.json()).id;
    await page.goto("/settings/plugins?tab=browse&extension=STL&keep=fixture");
    const chip = page.getByRole("button", { name: "File type: stl (clear)", exact: true });
    await expect(chip).toBeVisible();
    await expect(page.getByText("STL filter fixture", { exact: true })).toBeVisible();
    await expect(page.getByText("OBJ filter fixture", { exact: true })).toHaveCount(0);
    await chip.click();
    await expect(chip).toHaveCount(0);
    expect(new URL(page.url()).searchParams.has("extension")).toBe(false);
    expect(new URL(page.url()).searchParams.get("tab")).toBe("browse");
    expect(new URL(page.url()).searchParams.get("keep")).toBe("fixture");
    await expect(page.getByText("OBJ filter fixture", { exact: true })).toBeVisible();
    await page.reload();
    await expect(chip).toHaveCount(0);
    await expect(page.getByText("STL filter fixture", { exact: true })).toBeVisible();
    await expect(page.getByText("OBJ filter fixture", { exact: true })).toBeVisible();
    await page.goto("/settings/plugins?tab=browse&extension=zzviewerfixture");
    await expect(
      page.getByText("No plugins match your search. Try clearing the filters.", { exact: true }),
    ).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    try {
      if (sourceId)
        expect(
          (await api.rawRequest("DELETE", `/api/plugins/marketplace/sources/${sourceId}`)).ok,
        ).toBe(true);
    } finally {
      server.closeAllConnections();
      if (server.listening)
        await new Promise<void>((resolve, reject) =>
          server.close((error) => (error ? reject(error) : resolve())),
        );
    }
  }
}
