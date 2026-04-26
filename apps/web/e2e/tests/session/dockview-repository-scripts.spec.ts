import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function seedTaskWithSession(
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

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

/** Read the visible top-level menu items in the dockview "+" dropdown. */
async function readMenuItemTexts(testPage: Page): Promise<string[]> {
  return testPage.locator('[role="menuitem"]:visible').allInnerTexts();
}

/**
 * Read every xterm buffer on the page and return their concatenated text. Used
 * to assert that *some* terminal contains given output regardless of which
 * dockview group / tab order it ended up in.
 */
async function readAllTerminalBuffers(testPage: Page): Promise<string> {
  return testPage.evaluate(() => {
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const panels = Array.from(document.querySelectorAll('[data-testid="terminal-panel"]'));
    return panels
      .map((panel) => {
        const xtermEl = panel.querySelector(".xterm");
        const container = xtermEl?.parentElement as XC | null | undefined;
        return container?.__xtermReadBuffer?.() ?? "";
      })
      .join("\n----\n");
  });
}

test.describe("Dockview repository scripts in + menu", () => {
  test.describe.configure({ retries: 1 });

  test("does not render Scripts section when repo has no scripts and no dev_script", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Ensure repo has no dev_script (seed doesn't set one, but be explicit).
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: "" });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "No Scripts");
    await session.addPanelButton().click();

    await expect(testPage.getByRole("menuitem", { name: "Terminal" })).toBeVisible();
    await expect(testPage.getByText("Scripts", { exact: true })).toHaveCount(0);
    await expect(testPage.getByTestId("run-dev-script")).toHaveCount(0);
  });

  test("renders custom scripts at the end of the + menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const buildScript = await apiClient.createRepositoryScript(
      seedData.repositoryId,
      "Run Build",
      "echo build",
      0,
    );
    const lintScript = await apiClient.createRepositoryScript(
      seedData.repositoryId,
      "Run Lint",
      "echo lint",
      1,
    );

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Custom Scripts");
    await session.addPanelButton().click();

    await expect(testPage.getByTestId(`run-script-${buildScript.id}`)).toBeVisible();
    await expect(testPage.getByTestId(`run-script-${lintScript.id}`)).toBeVisible();

    // Scripts must be the LAST items in the menu. Verify by ordering: the
    // "Browser" item appears before the script items in the visible menu.
    const items = await readMenuItemTexts(testPage);
    const browserIdx = items.findIndex((t) => t.includes("Browser"));
    const buildIdx = items.findIndex((t) => t.includes("Run Build"));
    const lintIdx = items.findIndex((t) => t.includes("Run Lint"));
    expect(browserIdx).toBeGreaterThanOrEqual(0);
    expect(buildIdx).toBeGreaterThan(browserIdx);
    expect(lintIdx).toBeGreaterThan(browserIdx);
  });

  test("clicking a custom script opens a terminal that runs the script command", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marker = "CUSTOM_SCRIPT_RAN_OK_3131";
    const script = await apiClient.createRepositoryScript(
      seedData.repositoryId,
      "Echo Hello",
      `echo ${marker}`,
      0,
    );

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Click Script");
    await session.addPanelButton().click();
    await testPage.getByTestId(`run-script-${script.id}`).click();

    // The backend assigns script terminals an id prefixed with "script-".
    await expect
      .poll(
        async () => {
          return testPage.evaluate(() => {
            type Panel = { id: string };
            type Api = { panels: Panel[] };
            const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
            return api?.panels.some((p) => p.id.startsWith("script-")) ?? false;
          });
        },
        { timeout: 10_000, message: "Expected a script terminal panel to be added" },
      )
      .toBe(true);

    // The script command must actually execute — *some* terminal buffer
    // (the one belonging to the new script panel) must contain the marker.
    await expect
      .poll(async () => (await readAllTerminalBuffers(testPage)).includes(marker), {
        timeout: 60_000,
        message: `Expected a terminal to contain "${marker}"`,
      })
      .toBe(true);
  });

  test("renders Dev Server entry when repository has dev_script", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateRepository(seedData.repositoryId, {
      dev_script: "echo dev-server-running",
    });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Dev Script");
    await session.addPanelButton().click();

    await expect(testPage.getByTestId("run-dev-script")).toBeVisible();
    await expect(testPage.getByText("Dev Server", { exact: true })).toBeVisible();

    // Also at the end of the menu — after Browser.
    const items = await readMenuItemTexts(testPage);
    const browserIdx = items.findIndex((t) => t.includes("Browser"));
    const devIdx = items.findIndex((t) => t.includes("Dev Server"));
    expect(devIdx).toBeGreaterThan(browserIdx);
  });

  test("dev_script and custom scripts coexist in the Scripts section", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: "echo dev" });
    const customScript = await apiClient.createRepositoryScript(
      seedData.repositoryId,
      "Test Suite",
      "echo test",
      0,
    );

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Mixed Scripts");
    await session.addPanelButton().click();

    await expect(testPage.getByTestId("run-dev-script")).toBeVisible();
    await expect(testPage.getByTestId(`run-script-${customScript.id}`)).toBeVisible();
  });

  test("clicking Dev Server opens a terminal that runs the dev_script command", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marker = "DEV_SCRIPT_RAN_OK_4242";
    await apiClient.updateRepository(seedData.repositoryId, {
      dev_script: `echo ${marker}`,
    });

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Run Dev Script");
    await session.addPanelButton().click();
    await testPage.getByTestId("run-dev-script").click();

    // Backend assigns "script-"-prefixed ids to script terminals.
    await expect
      .poll(
        async () => {
          return testPage.evaluate(() => {
            type Panel = { id: string };
            type Api = { panels: Panel[] };
            const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
            return api?.panels.some((p) => p.id.startsWith("script-")) ?? false;
          });
        },
        { timeout: 10_000, message: "Expected a dev-script terminal panel to be added" },
      )
      .toBe(true);

    // The dev_script must actually execute — *some* terminal buffer should
    // contain the marker emitted by the script command.
    await expect
      .poll(async () => (await readAllTerminalBuffers(testPage)).includes(marker), {
        timeout: 60_000,
        message: `Expected a terminal to contain "${marker}"`,
      })
      .toBe(true);
  });
});
