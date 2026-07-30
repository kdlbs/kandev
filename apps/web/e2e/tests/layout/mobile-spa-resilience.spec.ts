import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { ConsoleMessage, Page, Route } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const SETTINGS_URL = "/settings/system/updates";
const SETTINGS_CHUNK_GLOB = "**/assets/settings-routes-*.js";
const SETTINGS_CHUNK_PATH = /^\/assets\/settings-routes-[A-Za-z0-9_-]+\.js$/;

type BrowserIssue = {
  kind: "console" | "pageerror";
  text: string;
  url: string;
};

function deferred() {
  let resolve = () => {};
  const promise = new Promise<void>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function assertSettingsChunk(route: Route) {
  const pathname = new URL(route.request().url()).pathname;
  expect(pathname).toMatch(SETTINGS_CHUNK_PATH);
  expect(pathname).not.toMatch(/\/(?:index|main)-[A-Za-z0-9_-]+\.js$/);
}

function watchBrowserIssues(page: Page) {
  const issues: BrowserIssue[] = [];
  page.on("pageerror", (error) => {
    issues.push({ kind: "pageerror", text: error.message, url: "" });
  });
  page.on("console", (message: ConsoleMessage) => {
    const text = message.text();
    const isCriticalWarning =
      /maximum update depth/i.test(text) || /result of getSnapshot should be cached/i.test(text);
    if (message.type() !== "error" && !isCriticalWarning) return;
    issues.push({
      kind: "console",
      text,
      url: message.location().url,
    });
  });
  return issues;
}

function expectNoUnexpectedIssues(
  issues: BrowserIssue[],
  allowConsole: (issue: BrowserIssue) => boolean = () => false,
) {
  const unexpected = issues.filter((issue) => issue.kind === "pageerror" || !allowConsole(issue));
  expect(unexpected, JSON.stringify(unexpected, null, 2)).toEqual([]);
}

function isExpectedPersistentChunkConsole(issue: BrowserIssue) {
  if (issue.kind !== "console") return false;
  const signal = `${issue.text}\n${issue.url}`;
  const failedSettingsChunk =
    /settings-routes-[A-Za-z0-9_-]+\.js/i.test(signal) &&
    /failed to load resource|dynamically imported module|module script/i.test(signal);
  const containedRouteFailure =
    /\[app\] route render failed/i.test(issue.text) &&
    /dynamically imported module|settings-routes-/i.test(issue.text);
  return failedSettingsChunk || containedRouteFailure;
}

function trackSettingsDocuments(page: Page) {
  let requests = 0;
  page.on("request", (request) => {
    if (
      request.isNavigationRequest() &&
      request.frame() === page.mainFrame() &&
      new URL(request.url()).pathname === SETTINGS_URL
    ) {
      requests += 1;
    }
  });
  return () => requests;
}

test.describe("Mobile SPA resilience", () => {
  test("shows accessible loading while the fingerprinted Settings chunk is delayed", async ({
    testPage,
  }) => {
    const issues = watchBrowserIssues(testPage);
    const chunkObserved = deferred();
    const releaseChunk = deferred();

    await testPage.route(SETTINGS_CHUNK_GLOB, async (route) => {
      assertSettingsChunk(route);
      chunkObserved.resolve();
      await releaseChunk.promise;
      await route.continue();
    });

    const navigation = testPage.goto(SETTINGS_URL);
    try {
      await chunkObserved.promise;
      await expect(
        testPage.getByRole("status").filter({ hasText: "Loading Settings…" }),
      ).toBeVisible();
    } finally {
      releaseChunk.resolve();
    }

    await navigation;
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");
    expectNoUnexpectedIssues(issues);
  });

  test("stops after one guarded reload and shows touch-safe route recovery", async ({
    testPage,
  }) => {
    const issues = watchBrowserIssues(testPage);
    const documentRequests = trackSettingsDocuments(testPage);
    let chunkRequests = 0;

    await testPage.route(SETTINGS_CHUNK_GLOB, async (route) => {
      assertSettingsChunk(route);
      chunkRequests += 1;
      await route.abort("failed");
    });

    await testPage.goto(SETTINGS_URL);
    const recovery = testPage.getByRole("alert").filter({ hasText: "This page couldn’t load." });
    await expect(recovery).toBeVisible();

    const reloadAction = recovery.getByRole("button", { name: "Reload" });
    await expect(reloadAction).toBeVisible();
    const reloadBox = await reloadAction.boundingBox();
    expect(reloadBox, "Reload action has no rendered hitbox").not.toBeNull();
    expect(reloadBox?.height ?? 0).toBeGreaterThanOrEqual(44);

    await assertNoDocumentHorizontalOverflow(testPage, "mobile route recovery");
    await expect(testPage.locator("#root")).toContainText("This page couldn’t load.");
    expect(chunkRequests).toBe(2);
    expect(documentRequests()).toBe(2);
    expectNoUnexpectedIssues(issues, isExpectedPersistentChunkConsole);
  });

  test("opens a multi-repository picker when optional task hydration fails", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const secondaryRepoDir = path.join(backend.tmpDir, "repos", "mobile-resilience-secondary");
    fs.mkdirSync(secondaryRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: secondaryRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: secondaryRepoDir, env: gitEnv });
    const secondaryRepo = await apiClient.createRepository(
      seedData.workspaceId,
      secondaryRepoDir,
      "main",
      { name: "Mobile resilience secondary" },
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile optional hydration resilience",
      seedData.agentProfileId,
      {
        description: 'e2e:message("mobile optional hydration response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId, secondaryRepo.id],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("multi-repository task has no session");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await expect(mobile.taskCard(task.id)).toBeVisible();

    const issues = watchBrowserIssues(testPage);
    const releaseFailures = deferred();
    const repositoryObserved = deferred();
    const repositorySettled = deferred();
    const sessionObserved = deferred();
    const sessionSettled = deferred();
    let repositoryFailures = 0;
    let sessionFailures = 0;
    const repositoryPath = `/api/v1/workspaces/${seedData.workspaceId}/repositories`;
    const sessionPath = `/api/v1/task-sessions/${task.session_id}`;
    const repositoryPattern = new RegExp(
      `${repositoryPath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\?include_scripts=true$`,
    );
    const sessionPattern = new RegExp(`${sessionPath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`);

    const failRepositories = async (route: Route) => {
      repositoryFailures += 1;
      repositoryObserved.resolve();
      await releaseFailures.promise;
      try {
        await route.fulfill({ status: 503, contentType: "application/json", body: "{}" });
      } finally {
        repositorySettled.resolve();
      }
    };
    const failSession = async (route: Route) => {
      sessionFailures += 1;
      sessionObserved.resolve();
      await releaseFailures.promise;
      try {
        await route.fulfill({ status: 503, contentType: "application/json", body: "{}" });
      } finally {
        sessionSettled.resolve();
      }
    };

    await testPage.route(repositoryPattern, failRepositories);
    await testPage.route(sessionPattern, failSession);
    try {
      await mobile.taskCard(task.id).tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
      await Promise.all([repositoryObserved.promise, sessionObserved.promise]);
      await expect(testPage.getByRole("status").filter({ hasText: "Loading task…" })).toBeVisible();

      releaseFailures.resolve();
      await Promise.all([repositorySettled.promise, sessionSettled.promise]);

      const repoPill = testPage.getByTestId("mobile-repo-pill");
      await expect(repoPill).toBeVisible();
      await repoPill.tap();

      const picker = testPage.getByRole("dialog");
      await expect(picker.getByText("Repositories", { exact: true })).toBeVisible();
      await expect(testPage.getByTestId(`mobile-repo-row-${seedData.repositoryId}`)).toBeVisible();
      await expect(testPage.getByTestId(`mobile-repo-row-${secondaryRepo.id}`)).toBeVisible();
      await expect(testPage.locator("#root")).not.toBeEmpty();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile repository picker recovery");

      expect(repositoryFailures).toBe(1);
      expect(sessionFailures).toBe(1);
      expectNoUnexpectedIssues(issues, (issue) => {
        if (issue.kind !== "console") return false;
        const signal = `${issue.text}\n${issue.url}`;
        return (
          (signal.includes(repositoryPath) || signal.includes(sessionPath)) &&
          /failed to load resource|503|service unavailable/i.test(signal)
        );
      });
    } finally {
      releaseFailures.resolve();
      await testPage.unroute(repositoryPattern, failRepositories);
      await testPage.unroute(sessionPattern, failSession);
    }
  });
});
