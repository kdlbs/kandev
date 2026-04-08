import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function seedSimpleTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

test.describe("File tree sorting", () => {
  test.describe.configure({ retries: 1 });

  test("directories appear before files inside nested folders, including dotfiles", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Seed a nested folder containing a mix of dirs (incl. a dot-dir) and files
    // (incl. dotfiles). If the sort is wrong, dotfiles will appear above .vscode.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const subDir = path.join(repoDir, "sub");

    fs.mkdirSync(path.join(subDir, "adir"), { recursive: true });
    fs.writeFileSync(path.join(subDir, "adir", "keep.txt"), "x\n");
    fs.mkdirSync(path.join(subDir, ".vscode"), { recursive: true });
    fs.writeFileSync(path.join(subDir, ".vscode", "settings.json"), "{}\n");
    fs.writeFileSync(path.join(subDir, ".gitignore"), "node_modules\n");
    fs.writeFileSync(path.join(subDir, ".golangci.yaml"), "linters:\n");
    fs.writeFileSync(path.join(subDir, "zfile.txt"), "z\n");

    const session = await seedSimpleTask(testPage, apiClient, seedData, "File Tree Sorting");

    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });

    // Expand the sub folder
    const subRow = session.files.locator('[data-testid="file-tree-node"][data-path="sub"]');
    await expect(subRow).toBeVisible({ timeout: 10_000 });
    await subRow.click();

    // Wait until all expected children are rendered
    const childSelector = (p: string) =>
      session.files.locator(`[data-testid="file-tree-node"][data-path="${p}"]`);

    await expect(childSelector("sub/adir")).toBeVisible({ timeout: 10_000 });
    await expect(childSelector("sub/.vscode")).toBeVisible({ timeout: 10_000 });
    await expect(childSelector("sub/.gitignore")).toBeVisible({ timeout: 10_000 });
    await expect(childSelector("sub/.golangci.yaml")).toBeVisible({ timeout: 10_000 });
    await expect(childSelector("sub/zfile.txt")).toBeVisible({ timeout: 10_000 });

    // Read the rendered order of sub's direct children
    const childPaths = await session.files
      .locator('[data-testid="file-tree-node"]')
      .evaluateAll((els) =>
        els
          .map((el) => ({
            path: el.getAttribute("data-path") ?? "",
            isDir: el.getAttribute("data-is-dir") === "true",
          }))
          .filter((n) => n.path.startsWith("sub/") && !n.path.slice("sub/".length).includes("/")),
      );

    // Directories must come before files. Within each group, order follows
    // the comparator's localeCompare (dot-prefixed names sort before letters).
    const dirs = childPaths.filter((n) => n.isDir).map((n) => n.path);
    const files = childPaths.filter((n) => !n.isDir).map((n) => n.path);

    expect(dirs).toEqual(["sub/.vscode", "sub/adir"]);
    expect(files).toEqual(["sub/.gitignore", "sub/.golangci.yaml", "sub/zfile.txt"]);
  });
});
