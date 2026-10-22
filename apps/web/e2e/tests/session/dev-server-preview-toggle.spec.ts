import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { waitForHttp } from "../../helpers/causal-waits";

const DEV_OUTPUT_MARKER = "kandev-dev-script-marker";
const START_LABEL = "Start dev server preview";
const STOP_LABEL = "Stop dev server preview";

/** Read the dev-server panel's xterm buffer. */
async function readDevServerBuffer(testPage: Page): Promise<string> {
  return testPage.evaluate(() => {
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const panel = document.querySelector('[data-testid="dev-server-panel"]');
    const container = panel?.querySelector(".xterm")?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

async function seedDevScriptSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  await apiClient.updateRepository(seedData.repositoryId, {
    dev_script: `echo ${DEV_OUTPUT_MARKER}; sleep 600`,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Dev server preview toggle",
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
  await session.waitForChatIdle();
  return session;
}

test.describe("dev server preview control", () => {
  test.describe.configure({ retries: 1 });

  let originalDevScript: string;
  test.beforeEach(async ({ testPage, apiClient, seedData }) => {
    void testPage;
    originalDevScript = (await apiClient.getRepository(seedData.repositoryId)).dev_script ?? "";
  });
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: originalDevScript });
  });

  test("starts the dev script, shows its output, and offers a stop control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedDevScriptSession(testPage, apiClient, seedData);

    const toggle = testPage.getByTestId("dev-server-preview-toggle");
    await expect(toggle).toHaveAttribute("aria-label", START_LABEL);

    const started = waitForHttp(testPage, "POST", /\/processes\/start$/);
    await toggle.click();
    await started;

    // The dev script's stdout is visible without leaving the task page.
    await expect(testPage.getByTestId("dev-server-panel")).toBeVisible();
    await expect
      .poll(async () => (await readDevServerBuffer(testPage)).includes(DEV_OUTPUT_MARKER), {
        timeout: 60_000,
        message: "dev script output never reached the Dev Server panel",
      })
      .toBe(true);

    // The same control now stops the process it started.
    await expect(toggle).toHaveAttribute("aria-label", STOP_LABEL);

    const stopped = waitForHttp(testPage, "POST", /\/processes\/[^/]+\/stop$/);
    await toggle.click();
    await stopped;

    await expect(toggle).toHaveAttribute("aria-label", START_LABEL);
  });
});

type HeldStart = { session: string; release: () => void; finished: Promise<void> };

async function holdStarts(testPage: Page, held: HeldStart[]) {
  // Synthetic transport keeps this scenario independent of process startup.
  await testPage.route("**/processes/start", async (route) => {
    let release!: () => void;
    let finish!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const finished = new Promise<void>((resolve) => {
      finish = resolve;
    });
    const sessionId = new URL(route.request().url()).pathname.split("/").at(-3)!;
    held.push({ session: sessionId, release, finished });
    try {
      await gate;
      await route.fulfill({
        json: {
          process: {
            id: `preview-${sessionId}`,
            session_id: sessionId,
            kind: "dev",
            status: "running",
            command: "echo preview",
            working_dir: "/repo",
            started_at: "2026-10-21T12:00:00Z",
            updated_at: "2026-10-21T12:00:00Z",
          },
        },
      });
    } finally {
      finish();
    }
  });
}

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.17
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.18
test("keeps both headers pending only for their selected session during navigation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const original = (await apiClient.getRepository(seedData.repositoryId)).dev_script ?? "";
  const held: HeldStart[] = [];
  try {
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: "echo preview" });
    const create = (title: string) =>
      apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
    const a = await create("Preview pending Alpha");
    const b = await create("Preview pending Beta");
    await testPage.goto(`/t/${a.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const center = testPage.getByTestId("dev-server-preview-toggle-center");
    const terminal = testPage.getByTestId("dev-server-preview-toggle");
    await expect(center).toBeVisible();
    await expect(terminal).toBeVisible();

    await holdStarts(testPage, held);
    const aRequest = testPage.waitForRequest(`**/task-sessions/${a.session_id}/processes/start`);
    await center.click();
    await aRequest;
    await expect(center).toBeDisabled();
    await expect(terminal).toBeDisabled();
    await session.clickTaskInSidebar("Preview pending Beta");
    await expect(testPage).toHaveURL(new RegExp(`/t/${b.id}`));
    await expect(center).toBeEnabled();
    await expect(terminal).toBeEnabled();
    const bRequest = testPage.waitForRequest(`**/task-sessions/${b.session_id}/processes/start`);
    await terminal.click();
    await bRequest;
    await expect(center).toBeDisabled();
    await expect(terminal).toBeDisabled();
    const aResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/task-sessions/${a.session_id}/processes/start$`),
    );
    held.find((item) => item.session === a.session_id)!.release();
    await aResponse;
    await expect(center).toBeDisabled();
    await expect(terminal).toBeDisabled();
    await session.clickTaskInSidebar("Preview pending Alpha");
    await expect(center).toBeEnabled();
    await session.clickTaskInSidebar("Preview pending Beta");
    await expect(center).toBeDisabled();
    const bResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/task-sessions/${b.session_id}/processes/start$`),
    );
    held.find((item) => item.session === b.session_id)!.release();
    await bResponse;
    await expect(center).toBeEnabled();
    await expect(terminal).toBeEnabled();
  } finally {
    for (const request of held) request.release();
    await Promise.all(held.map((request) => request.finished));
    await testPage.unrouteAll({ behavior: "wait" });
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: original });
  }
});
