import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import { JiraSettingsPage } from "../../pages/jira-settings-page";

function snapshot(repository: string) {
  return {
    head: execFileSync("git", ["-C", repository, "rev-parse", "HEAD"], { encoding: "utf8" }),
    status: execFileSync(
      "git",
      ["-C", repository, "status", "--porcelain=v1", "--untracked-files=all"],
      { encoding: "utf8" },
    ),
  };
}

// @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4
export async function exerciseJiraRefresh({
  page,
  api,
  seedRepository,
  runtimeDirectory,
  testInfo,
  touch,
}: {
  page: Page;
  api: ApiClient;
  seedRepository: string;
  runtimeDirectory: string;
  testInfo: TestInfo;
  touch: boolean;
}) {
  const seedBefore = snapshot(seedRepository);
  const sourceRoot = path.resolve(process.cwd(), "../..");
  const sourceBefore = snapshot(sourceRoot);
  const repository = mkdtempSync(path.join(runtimeDirectory, "jira-refresh-"));
  const name = `Jira refresh ${testInfo.testId}`;
  let workspace: { id: string } | undefined;
  let executor: { id: string } | undefined;
  let repositoryId: string | undefined;
  const release = Promise.withResolvers<void>();
  const captured = Promise.withResolvers<void>();
  const held: Promise<void>[] = [];
  let holding = true;
  const configPattern = "**/api/v1/jira/config?workspace_id=*";
  try {
    execFileSync("git", ["init", "-b", "main", repository]);
    writeFileSync(path.join(repository, "fixture.txt"), "Disposable Jira settings fixture\n");
    execFileSync("git", ["-C", repository, "add", "fixture.txt"]);
    execFileSync("git", [
      "-C",
      repository,
      "-c",
      "user.name=Jira Fixture",
      "-c",
      "user.email=jira@example.test",
      "commit",
      "-m",
      "test: seed disposable Jira repository",
    ]);
    workspace = await api.createWorkspace(name);
    executor = await api.createExecutor(name, "worktree");
    await api.updateWorkspace(workspace.id, { default_executor_id: executor.id });
    repositoryId = (await api.createRepository(workspace.id, repository)).id;
    if (!touch) {
      await api.setJiraConfig({
        workspaceId: workspace.id,
        siteUrl: "https://fixture.atlassian.net",
        email: "fixture@example.test",
        secret: "fixture-only-token",
        defaultProjectKey: "BASE",
      });
    }
    await page.clock.install();
    const settings = new JiraSettingsPage(page);
    await settings.gotoWorkspace(workspace.id);
    await expect(settings.projectInput).toHaveValue(touch ? "" : "BASE");
    await expect(settings.projectInput).toBeEnabled();
    const workspaceId = workspace.id;
    await page.route(configPattern, async (route) => {
      if (
        route.request().method() !== "GET" ||
        new URL(route.request().url()).searchParams.get("workspace_id") !== workspaceId ||
        !holding
      )
        return route.continue();
      const pending = (async () => {
        try {
          const response = await route.fetch();
          expect(response.status()).toBe(touch ? 204 : 200);
          captured.resolve();
          await release.promise;
          await route.fulfill({ response });
        } catch (error) {
          captured.reject(error);
          throw error;
        }
      })();
      held.push(pending);
      return pending;
    });
    await page.clock.fastForward(90_000);
    await captured.promise;
    if (touch) {
      await settings.fillForm({
        siteUrl: "https://fixture.atlassian.net",
        email: "fixture@example.test",
        secret: "fixture-only-token",
        projectKey: "SAVED",
      });
    } else await settings.projectInput.fill("SAVED");
    await expect(settings.saveButton).toBeEnabled();
    const saved = waitForHttp(page, "POST", /\/api\/v1\/jira\/config$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === workspaceId,
    });
    if (touch) await settings.saveButton.tap();
    else await settings.saveButton.click();
    const saveResponse = await saved;
    expect(saveResponse.status()).toBe(200);
    await saveResponse.finished();
    await expect(settings.projectInput).toHaveAttribute("data-settings-dirty", "false");
    const staleRead = waitForHttp(page, "GET", /\/api\/v1\/jira\/config$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === workspaceId,
    });
    holding = false;
    release.resolve();
    await Promise.all(held);
    const staleResponse = await staleRead;
    expect(staleResponse.status()).toBe(touch ? 204 : 200);
    // A 204 has no response body to await.
    if (staleResponse.status() !== 204) await staleResponse.finished();
    await dwell(
      page,
      200,
      "negative-assertion",
      "Rejected Jira read publication emits no event; observe that the settled stale response cannot dirty the saved form.",
    );
    await expect(settings.projectInput).toHaveValue("SAVED");
    await expect(settings.projectInput).toHaveAttribute("data-settings-dirty", "false");
    await expect(settings.saveButton).toHaveCount(0);
    await settings.projectInput.fill("NEWER");
    const reset = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset", exact: true });
    if (touch) await reset.tap();
    else await reset.click();
    await expect(settings.projectInput).toHaveValue("SAVED");
    await expect(settings.projectInput).toHaveAttribute("data-settings-dirty", "false");
    await settings.projectInput.fill("NEXT");
    const savedAgain = waitForHttp(page, "POST", /\/api\/v1\/jira\/config$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === workspaceId,
    });
    if (touch) await settings.saveButton.tap();
    else await settings.saveButton.click();
    const secondResponse = await savedAgain;
    expect(secondResponse.status()).toBe(200);
    await secondResponse.finished();
    await expect(settings.projectInput).toHaveAttribute("data-settings-dirty", "false");
    await page.reload();
    await expect(settings.projectInput).toHaveValue("NEXT");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await settings.projectInput.scrollIntoViewIfNeeded();
    const screenshot = testInfo.outputPath("jira-refresh-saved.png");
    await page.screenshot({ path: screenshot });
    await testInfo.attach("jira-refresh-saved", {
      path: screenshot,
      contentType: "image/png",
    });
  } finally {
    holding = false;
    release.resolve();
    try {
      await Promise.all(held);
    } finally {
      try {
        if (!page.isClosed()) {
          await page.unrouteAll({ behavior: "wait" });
          await page.goto("/settings");
        }
      } finally {
        if (repositoryId) await api.deleteRepository(repositoryId);
        if (workspace) await api.deleteWorkspace(workspace.id, name);
        if (executor) await api.deleteExecutor(executor.id);
        rmSync(repository, { recursive: true, force: true });
        expect(snapshot(seedRepository)).toEqual(seedBefore);
        expect(snapshot(sourceRoot)).toEqual(sourceBefore);
      }
    }
  }
}
