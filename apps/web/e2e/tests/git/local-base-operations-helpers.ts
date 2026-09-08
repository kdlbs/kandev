import { expect, type Locator, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import { restoreSeedRepositoryOrigin, type SeedData } from "../../fixtures/test-base";
import type { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

export type LocalBaseOperation = "merge" | "rebase";

export type LocalBaseScenario = {
  git: GitHelper;
  branch: string;
  baseHead: string;
};

function seedRepositoryGit(seedData: SeedData, backend: BackendContext): GitHelper {
  return new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
}

/** Return the worker fixture repository to its seeded origin/main state. */
export function restoreLocalBaseRepository(seedData: SeedData, backend: BackendContext): void {
  const git = seedRepositoryGit(seedData, backend);
  for (const command of ["git rebase --abort", "git merge --abort"]) {
    try {
      git.exec(command);
    } catch {
      // No operation is in progress for the normal cleanup path.
    }
  }
  git.exec("git checkout -f main");
  git.exec("git clean -fd");
  for (const branch of ["feature/local-only-merge", "feature/local-only-rebase"]) {
    try {
      git.exec(`git branch -D ${branch}`);
    } catch {
      // The branch may not have been created if setup failed early.
    }
  }
  restoreSeedRepositoryOrigin(seedData);
  git.exec("git fetch --no-tags origin");
  git.exec("git reset --hard origin/main");
  git.exec("git clean -fd");
}

/**
 * Build a feature branch and a newer local main, then remove origin. The task
 * is created against the returned branch so the UI operation must choose the
 * local refs/heads/main target.
 */
export function prepareLocalBaseScenario(
  seedData: SeedData,
  backend: BackendContext,
  operation: LocalBaseOperation,
): LocalBaseScenario {
  restoreLocalBaseRepository(seedData, backend);
  const git = seedRepositoryGit(seedData, backend);
  const branch = `feature/local-only-${operation}`;

  git.exec(`git checkout -B ${branch} main`);
  git.createFile(`local-only-${operation}-feature.txt`, `${operation} feature\n`);
  git.stageAll();
  git.commit(`local-only ${operation} feature`);

  git.exec("git checkout main");
  git.createFile(`local-only-${operation}-base.txt`, `${operation} base\n`);
  git.stageAll();
  const baseHead = git.commit(`local-only ${operation} base`);

  git.exec(`git checkout ${branch}`);
  git.exec("git remote remove origin");
  return { git, branch, baseHead };
}

export async function openDesktopVcsMenu(session: SessionPage, page: Page): Promise<Locator> {
  await session.clickTab("Changes");
  const changes = page.getByTestId("changes-panel");
  await expect(changes.getByRole("button", { name: "Review" })).toBeVisible({ timeout: 15_000 });
  await changes.getByRole("button", { name: "Review" }).click();

  const reviewDialog = page.getByRole("dialog", { name: "Review Changes" });
  await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
  await reviewDialog.getByRole("button", { name: "Open VCS options" }).click();

  const menu = page.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
  await expect(menu).toBeVisible();
  return menu;
}

export async function waitForGitSuccess(page: Page, operation: "Merge" | "Rebase"): Promise<void> {
  await expect(
    page.getByTestId("toast-message").filter({ hasText: `${operation} successful` }),
  ).toBeVisible({
    timeout: 15_000,
  });
}

export function assertBaseCommitReachable(git: GitHelper, baseHead: string): void {
  expect(git.exec(`git merge-base --is-ancestor ${baseHead} HEAD`).trim()).toBe("");
}
