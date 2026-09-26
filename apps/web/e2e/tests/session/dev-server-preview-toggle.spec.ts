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
