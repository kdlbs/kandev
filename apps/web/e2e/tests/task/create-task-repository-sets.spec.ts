import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { useRegularMode } from "../../helpers/regular-mode";

// Exercises the regular task-create dialog (New Task in the sidebar), so run
// with the office feature disabled.
useRegularMode();

const SET_OPTION = "repository-set-option";
const REPO_CHIP_TRIGGER = "repo-chip-trigger";
const SECOND_REPO_NAME = "Repository Sets Target";
const SET_NAME = "Full-stack";

type TaskWithRepos = {
  repositories?: Array<{ repository_id: string; base_branch?: string }>;
};

async function openRepositorySetPicker(testPage: Page, dialog: Locator) {
  await dialog.getByTestId("add-repository").click();
  const menu = testPage.getByTestId("workspace-source-menu-options");
  await expect(menu).toBeVisible();
  await menu.getByTestId("workspace-source-menu-set").click();
  await expect(testPage.getByTestId("workspace-source-set-picker")).toBeVisible();
}

/**
 * Seeds a second workspace repository plus a set containing both, and returns
 * both repository ids so a test can assert the created task's membership.
 */
async function seedSetWithTwoRepositories(
  apiClient: {
    createRepository: (
      workspaceId: string,
      localPath: string,
      defaultBranch?: string,
      opts?: { name?: string },
    ) => Promise<{ id: string }>;
    createRepositorySet: (
      workspaceId: string,
      name: string,
      repositoryIds: string[] | Array<{ repositoryId: string; baseBranch?: string }>,
    ) => Promise<{ id: string; name: string }>;
  },
  seedData: { workspaceId: string; repositoryId: string },
  backend: { tmpDir: string },
  options: { setName?: string; dirName?: string; savedBases?: boolean } = {},
) {
  const dir = path.join(backend.tmpDir, "repos", options.dirName ?? "repository-sets-target");
  fs.mkdirSync(dir, { recursive: true });
  const gitEnv = makeGitEnv(backend.tmpDir);
  execSync("git init -b main", { cwd: dir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: dir, env: gitEnv });
  execSync("git branch develop", { cwd: dir, env: gitEnv });
  const second = await apiClient.createRepository(seedData.workspaceId, dir, "main", {
    name: SECOND_REPO_NAME,
  });
  const members: string[] | Array<{ repositoryId: string; baseBranch?: string }> =
    options.savedBases
      ? [
          { repositoryId: seedData.repositoryId, baseBranch: "main" },
          { repositoryId: second.id, baseBranch: "develop" },
        ]
      : [seedData.repositoryId, second.id];
  const set = await apiClient.createRepositorySet(
    seedData.workspaceId,
    options.setName ?? SET_NAME,
    members,
  );
  return { secondRepositoryId: second.id, setId: set.id };
}

test.describe("Task creation with repository sets", () => {
  test("applying a set fills the picker and the created task carries both repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const { secondRepositoryId } = await seedSetWithTwoRepositories(apiClient, seedData, backend, {
      savedBases: true,
    });

    await testPage.goto("/");
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Repository sets are available from the unified Add menu.
    await openRepositorySetPicker(testPage, dialog);
    const options = testPage.getByTestId(SET_OPTION);
    await expect(options).toHaveCount(1);
    await expect(options.first()).toContainText(SET_NAME);
    await expect(options.first()).toHaveCSS("font-size", "12px");
    await expect(options.first()).toHaveCSS("min-height", "28px");
    await expect(options.first().getByText(SET_NAME, { exact: true })).toHaveCSS(
      "font-size",
      "12px",
    );
    await expect(
      testPage
        .getByTestId("workspace-source-view-set")
        .getByText("Repository Set", { exact: true }),
    ).toHaveCSS("font-size", "12px");
    await options.first().click();

    // One row per member, in set order.
    const chips = dialog.getByTestId(REPO_CHIP_TRIGGER);
    await expect(chips).toHaveCount(2);
    await expect(chips.nth(1)).toContainText(SECOND_REPO_NAME);
    await expect(dialog.getByTestId("repo-chip").nth(1)).toContainText("develop");
    await prCapture.screenshot("desktop-repository-set-applied", {
      caption: "Applying a repository set on desktop fills the task picker with saved bases.",
    });

    const title = `Repository set task ${Date.now()}`;
    await dialog.getByTestId("task-title-input").fill(title);
    await dialog.getByTestId("task-description-input").fill("Created from a repository set");
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog).not.toBeVisible();

    type TaskEntry = { id: string; title: string };
    let created: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const all = await apiClient.listTasks(seedData.workspaceId);
          created = all.tasks.find((entry: TaskEntry) => entry.title === title);
          return created;
        },
        { message: "the task created from a set should exist" },
      )
      .toBeDefined();

    const raw = await apiClient.rawRequest("GET", `/api/v1/tasks/${created!.id}`);
    const data = (await raw.json()) as TaskWithRepos;
    const repoIds = data.repositories?.map((entry) => entry.repository_id) ?? [];
    expect(repoIds).toEqual(expect.arrayContaining([seedData.repositoryId, secondRepositoryId]));
    expect(repoIds).toHaveLength(2);
    expect(
      data.repositories?.find((entry) => entry.repository_id === secondRepositoryId)?.base_branch,
    ).toBe("develop");
  });

  test("applying the same set twice adds no duplicate rows", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await seedSetWithTwoRepositories(apiClient, seedData, backend, {
      dirName: "repository-sets-idempotent",
    });

    await testPage.goto("/");
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    while ((await dialog.getByTestId(REPO_CHIP_TRIGGER).count()) > 0) {
      await dialog.getByTestId("remove-repo-chip").first().click();
    }
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER)).toHaveCount(0);
    await openRepositorySetPicker(testPage, dialog);
    await testPage.getByTestId(SET_OPTION).first().click();
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER)).toHaveCount(2);

    await openRepositorySetPicker(testPage, dialog);
    const option = testPage.getByTestId(SET_OPTION).first();
    await option.click();

    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER)).toHaveCount(2);
  });

  test("with no sets the menu offers only Save as set", async ({ testPage, seedData }) => {
    expect(seedData.workspaceId).toBeTruthy();

    await testPage.goto("/");
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER).first()).toBeVisible();

    // The control stays reachable so the first set can be defined from the flow
    // that just chose the repositories, but it lists no sets to apply.
    await openRepositorySetPicker(testPage, dialog);
    await expect(testPage.getByTestId("repository-set-save-action")).toBeVisible();
    await expect(testPage.getByTestId(SET_OPTION)).toHaveCount(0);
  });

  test("the Sets control stays available after the last row is removed", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await seedSetWithTwoRepositories(apiClient, seedData, backend, {
      setName: "Round trip set",
      dirName: "repository-sets-mode-round-trip",
    });

    await testPage.goto("/");
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const add = dialog.getByTestId("add-repository");
    const row = dialog.getByTestId("repo-chips-row");
    await expect(add).toBeEnabled();

    // Sets remain available for an empty editable draft. Removing the final
    // row must not make the Add menu disappear or restore a row asynchronously.
    while ((await dialog.getByTestId(REPO_CHIP_TRIGGER).count()) > 0) {
      await dialog.getByTestId("remove-repo-chip").first().click();
    }
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER)).toHaveCount(0);
    await expect(add).toBeEnabled();
    await expect(row).not.toContainText("Multi-repo tasks are unavailable");

    await openRepositorySetPicker(testPage, dialog);
    await expect(testPage.getByTestId(SET_OPTION).first()).toBeVisible();
    await testPage.getByTestId(SET_OPTION).first().click();
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER)).toHaveCount(2);
  });

  test("saving the current selection creates a set the backend can list", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // One existing set so the Sets menu (and its "Save as set" action) is offered.
    await seedSetWithTwoRepositories(apiClient, seedData, backend, {
      setName: "Seeded set",
      dirName: "repository-sets-save",
    });

    await testPage.goto("/");
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER).first()).toBeVisible();

    await openRepositorySetPicker(testPage, dialog);
    await testPage.getByTestId("repository-set-save-action").click();

    const savedName = `Saved selection ${Date.now()}`;
    await testPage.getByTestId("repository-set-name").fill(savedName);
    await testPage.getByTestId("repository-set-save-submit").click();

    await expect
      .poll(
        async () => {
          const listed = await apiClient.listRepositorySets(seedData.workspaceId);
          return listed.repository_sets.map((entry) => entry.name);
        },
        { message: "the saved selection should appear as a new set" },
      )
      .toContain(savedName);

    // The in-progress task is untouched: the dialog is still open with its rows.
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId(REPO_CHIP_TRIGGER).first()).toBeVisible();
  });
});
