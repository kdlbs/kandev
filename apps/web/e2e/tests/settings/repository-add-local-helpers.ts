import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import type { ListRepositoriesResponse, Repository } from "../../../lib/types/http";

export async function addExplicitLocalRepository(options: {
  page: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
  mobile?: boolean;
}) {
  const { page, apiClient, backend, seedData, mobile = false } = options;
  if (mobile) await page.setViewportSize({ width: 390, height: 844 });

  const externalRoot = fs.mkdtempSync(
    path.join(path.dirname(backend.tmpDir), "kandev-e2e-external-repository-"),
  );
  const repoName = mobile ? "outside-mobile-repository" : "outside-desktop-repository";
  const repoPath = path.join(externalRoot, repoName);
  const canonicalPath = fs.realpathSync(externalRoot);
  let savedRepository: Repository | undefined;

  try {
    fs.mkdirSync(repoPath, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoPath, env: gitEnv });
    fs.writeFileSync(path.join(repoPath, "README.md"), "# Explicit local repository\n");
    execSync("git add README.md", { cwd: repoPath, env: gitEnv });
    execSync('git commit -m "init explicit repository"', { cwd: repoPath, env: gitEnv });

    const relativeToBackendHome = path.relative(backend.tmpDir, repoPath);
    expect(relativeToBackendHome.startsWith(`..${path.sep}`)).toBe(true);

    await page.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
    await page.getByRole("button", { name: "Add Local Repository" }).click();

    const dialog = page.getByRole("dialog", { name: "Add Local Repository" });
    await expect(dialog).toBeVisible();
    const manualPath = dialog.getByPlaceholder("/absolute/path/to/repository");
    await manualPath.fill(repoPath);

    const validationResponse = page.waitForResponse(
      (response) =>
        response.url().includes("/repositories/validate?") &&
        response.request().method() === "GET" &&
        response.ok(),
    );
    await dialog.getByRole("button", { name: "Validate", exact: true }).click();
    await validationResponse;
    await expect(dialog.getByText("Valid git repository", { exact: true })).toBeVisible();

    const useRepository = dialog.getByRole("button", { name: "Use Repository" });
    await expect(useRepository).toBeEnabled();
    await useRepository.click();
    await expect(dialog).toBeHidden();

    await expect(page.getByPlaceholder("/path/to/repository")).toHaveValue(repoPath);
    const createResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/workspaces/${seedData.workspaceId}/repositories`) &&
        response.request().method() === "POST" &&
        response.ok(),
    );
    await page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    savedRepository = (await (await createResponse).json()) as Repository;

    await page.reload();
    const savedCard = page.locator('[data-slot="card"]', { hasText: repoName });
    await expect(savedCard).toBeVisible({ timeout: 15_000 });
    await expect(savedCard.getByText(fs.realpathSync(repoPath), { exact: true })).toBeVisible();

    const listResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
    );
    expect(listResponse.ok).toBe(true);
    const listed = (await listResponse.json()) as ListRepositoriesResponse;
    expect(listed.repositories).toContainEqual(
      expect.objectContaining({ id: savedRepository.id, local_path: fs.realpathSync(repoPath) }),
    );

    if (mobile) {
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);
    }
  } finally {
    if (savedRepository) {
      await apiClient
        .rawRequest("DELETE", `/api/v1/repositories/${savedRepository.id}`)
        .catch(() => undefined);
    }
    fs.rmSync(canonicalPath, { recursive: true, force: true });
  }
}
