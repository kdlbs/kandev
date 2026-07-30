import { expect, test } from "../../fixtures/test-base";
import type { ConsoleMessage, Page, Route } from "@playwright/test";

const SETTINGS_URL = "/settings/system/updates";
const SETTINGS_CHUNK_GLOB = "**/assets/settings-routes-*.js";
const SETTINGS_CHUNK_PATH = /^\/assets\/settings-routes-[A-Za-z0-9_-]+\.js$/;

type BrowserIssue = {
  kind: "console" | "pageerror";
  text: string;
  url: string;
};

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

function isExpectedChunkConsole(issue: BrowserIssue) {
  if (issue.kind !== "console") return false;
  const signal = `${issue.text}\n${issue.url}`;
  return (
    /settings-routes-[A-Za-z0-9_-]+\.js/i.test(signal) &&
    /failed to load resource|dynamically imported module|module script/i.test(signal)
  );
}

function isExpectedPersistentChunkConsole(issue: BrowserIssue) {
  return (
    isExpectedChunkConsole(issue) ||
    (issue.kind === "console" &&
      /\[app\] route render failed/i.test(issue.text) &&
      /dynamically imported module|settings-routes-/i.test(issue.text))
  );
}

test.describe("SPA route resilience", () => {
  test("shows accessible loading while the fingerprinted Settings chunk is delayed", async ({
    testPage,
  }) => {
    const issues = watchBrowserIssues(testPage);
    let releaseChunk = () => {};
    let observeChunk = () => {};
    const released = new Promise<void>((resolve) => {
      releaseChunk = resolve;
    });
    const observed = new Promise<void>((resolve) => {
      observeChunk = resolve;
    });

    await testPage.route(SETTINGS_CHUNK_GLOB, async (route) => {
      assertSettingsChunk(route);
      observeChunk();
      await released;
      await route.continue();
    });

    const navigation = testPage.goto(SETTINGS_URL);
    try {
      await observed;
      await expect(
        testPage.getByRole("status").filter({ hasText: "Loading Settings…" }),
      ).toBeVisible();
    } finally {
      releaseChunk();
    }
    await navigation;
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");
    expectNoUnexpectedIssues(issues);
  });

  test("reloads once after the first Settings chunk abort and then renders Settings", async ({
    testPage,
  }) => {
    const issues = watchBrowserIssues(testPage);
    const documentRequests = trackSettingsDocuments(testPage);
    let chunkRequests = 0;

    await testPage.route(SETTINGS_CHUNK_GLOB, async (route) => {
      assertSettingsChunk(route);
      chunkRequests += 1;
      if (chunkRequests === 1) {
        await route.abort("failed");
        return;
      }
      await route.continue();
    });

    await testPage.goto(SETTINGS_URL);
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");
    expect(chunkRequests).toBe(2);
    expect(documentRequests()).toBe(2);
    expectNoUnexpectedIssues(issues, isExpectedChunkConsole);
  });

  test("stops after one reload and shows route recovery when the Settings chunk stays unavailable", async ({
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
    await expect(recovery.getByRole("button", { name: "Reload" })).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar")).toBeVisible();
    expect(chunkRequests).toBe(2);
    expect(documentRequests()).toBe(2);
    expectNoUnexpectedIssues(issues, isExpectedPersistentChunkConsole);
  });
});
