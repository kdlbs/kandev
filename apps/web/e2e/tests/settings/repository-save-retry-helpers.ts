import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import { openAddLocalRepositoryDialog } from "../../helpers/add-repository-menu";
import type { Repository, RepositoryScript } from "../../../lib/types/http";

export async function repositorySaveRetry({
  page,
  testInfo,
  apiClient,
  backend,
  seedData,
  fresh,
  mobile = false,
}: {
  page: Page;
  testInfo: TestInfo;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
  fresh: boolean;
  mobile?: boolean;
}) {
  // This disposable checkout is independent of the shared seed repository.
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "save-retry-"));
  const name = `Retry ${fresh ? "new" : "existing"} ${mobile ? "phone" : "desktop"}`;
  const env = makeGitEnv(backend.tmpDir);
  execFileSync("git", ["init", "-b", "main"], { cwd: root, env });
  execFileSync("git", ["commit", "--allow-empty", "-m", "init"], { cwd: root, env });
  let registered: Repository | undefined;
  const posts: string[] = [];
  let rejectLint = true;
  const scriptsRoute = "**/api/v1/repositories/*/scripts";
  await page.route(scriptsRoute, async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    const body = route.request().postDataJSON() as { name: string };
    posts.push(body.name);
    if (body.name === "Lint" && rejectLint) {
      rejectLint = false;
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Lint rejected before persistence" }),
      });
      return;
    }
    await route.continue();
  });
  try {
    if (!fresh)
      registered = await apiClient.createRepository(seedData.workspaceId, root, "main", { name });
    await page.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    if (fresh) {
      const dialog = await openAddLocalRepositoryDialog(page);
      await dialog.getByPlaceholder("/absolute/path/to/repository").fill(root);
      await dialog.getByRole("button", { name: "Validate", exact: true }).click();
      await expect(dialog.getByText("Valid git repository", { exact: true })).toBeVisible();
      await dialog.getByRole("button", { name: "Use Repository", exact: true }).click();
      await expect(dialog).toBeHidden();
      await page.getByPlaceholder("my-repo").fill(name);
    } else {
      await page.getByRole("heading", { name, exact: true }).click();
    }
    for (const [index, script] of ["Build", "Lint"].entries()) {
      await page.getByRole("button", { name: "Add Script", exact: true }).click();
      await expect(page.getByPlaceholder("Script name")).toHaveCount(index + 1);
      await page.getByPlaceholder("Script name").nth(index).fill(script);
      const row = page.getByPlaceholder("Script name").nth(index).locator("../..");
      await row.locator("textarea").fill(`echo ${script}`);
    }
    const saveSurface = page.getByTestId("settings-floating-save");
    await saveSurface.getByRole("button", { name: "Save changes", exact: true }).click();
    await expect(saveSurface).toContainText("Couldn't save");
    await expect(page.getByPlaceholder("Script name")).toHaveCount(2);
    const list = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
    );
    expect(list.ok).toBe(true);
    const matches = ((await list.json()) as { repositories: Repository[] }).repositories.filter(
      (repo) => repo.name === name,
    );
    expect(matches).toHaveLength(1);
    registered = matches[0];
    const readScripts = async () => {
      const response = await apiClient.rawRequest(
        "GET",
        `/api/v1/repositories/${registered!.id}/scripts`,
      );
      expect(response.ok).toBe(true);
      return ((await response.json()) as { scripts: RepositoryScript[] }).scripts;
    };
    const partial = await readScripts();
    expect(partial.map((script) => script.name)).toEqual(["Build"]);
    const retry = saveSurface.getByRole("button", { name: "Retry save", exact: true });
    await expect(retry).toBeEnabled();
    await retry.click();
    await expect(saveSurface).toContainText("Saved");
    const complete = await readScripts();
    expect(complete.map((script) => script.name).sort()).toEqual(["Build", "Lint"]);
    expect(complete.find((script) => script.name === "Build")?.id).toBe(partial[0].id);
    expect(posts).toEqual(["Build", "Lint", "Lint"]);
    await page.reload();
    await expect(page.getByRole("heading", { name, exact: true })).toHaveCount(1);
    await page.getByRole("heading", { name, exact: true }).click();
    await expect(page.getByPlaceholder("Script name")).toHaveCount(2);
    await expect(page.getByPlaceholder("Script name").nth(0)).toHaveValue("Build");
    await expect(page.getByPlaceholder("Script name").nth(1)).toHaveValue("Lint");
    if (mobile) {
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);
      await testInfo.attach("phone-retry", {
        body: await page.screenshot({ fullPage: true }),
        contentType: "image/png",
      });
    }
  } finally {
    if (!page.isClosed()) await page.unroute(scriptsRoute);
    // Recover a known registration even if a UI assertion failed after creation.
    if (!registered) {
      const response = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
      );
      if (response.ok)
        registered = ((await response.json()) as { repositories: Repository[] }).repositories.find(
          (repo) => repo.local_path === root,
        );
    }
    if (registered) {
      const removed = await apiClient.rawRequest("DELETE", `/api/v1/repositories/${registered.id}`);
      expect(removed.ok).toBe(true);
    }
    fs.rmSync(root, { recursive: true, force: true });
  }
}
