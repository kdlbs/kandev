import { expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

const DIFFS_CONTAINER = "diffs-container";

type SeedTaskOptions = {
  title: string;
  scenarioCommand: string;
  completionText: string;
};

async function seedTaskWithScenario(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  options: SeedTaskOptions,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.title,
    seedData.agentProfileId,
    {
      description: options.scenarioCommand,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await closePreviewPanels(testPage);

  await expect(session.chat.getByText(options.completionText, { exact: false })).toBeVisible({
    timeout: 45_000,
  });

  return { session, sessionId: task.session_id };
}

export async function closePreviewPanels(testPage: Page) {
  await expect
    .poll(
      () =>
        testPage.evaluate(() => {
          type TestWindow = Window & {
            __dockviewApi__?: {
              getPanel: (id: string) => unknown;
              removePanel: (panel: unknown) => void;
            };
          };
          const dockview = (window as TestWindow).__dockviewApi__;
          if (!dockview) return false;
          for (const id of ["preview:file-diff", "preview:file-editor"]) {
            const panel = dockview.getPanel(id);
            if (panel) dockview.removePanel(panel);
          }
          return true;
        }),
      { timeout: 10_000 },
    )
    .toBe(true);
}

export function seedUntrackedFileTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Untracked File E2E",
    scenarioCommand: "/e2e:untracked-file-setup",
    completionText: "untracked-file-setup complete",
  });
}

export function seedDiffUpdateTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Diff Update E2E",
    scenarioCommand: "/e2e:diff-update-setup",
    completionText: "diff-update-setup complete",
  });
}

export function seedMultiFileTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Multi-file Diff Update E2E",
    scenarioCommand: "/e2e:multi-file-setup",
    completionText: "multi-file-setup complete",
  });
}

export async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

export async function openFileDiff(testPage: Page, fileName: string) {
  const fileRow = testPage.getByTestId(`file-row-${fileName.replace(/[/\\]/g, "-")}`);
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

export function fileDiffTab(testPage: Page, fileName: string) {
  return testPage.locator(".dv-default-tab[type='file-diff']", {
    hasText: `Diff [${fileName}]`,
  });
}

export function getDiffsContainer(testPage: Page) {
  return testPage.locator(DIFFS_CONTAINER);
}

export async function waitForDiffText(testPage: Page, text: string, timeout = 60_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((expected) => {
          const containers = Array.from(document.querySelectorAll("diffs-container")).filter(
            (el) => {
              const rect = el.getBoundingClientRect();
              const style = window.getComputedStyle(el);
              return (
                rect.width > 0 &&
                rect.height > 0 &&
                style.display !== "none" &&
                style.visibility !== "hidden"
              );
            },
          );
          return containers.some((container) =>
            container.shadowRoot?.textContent?.includes(expected),
          );
        }, text),
      { timeout },
    )
    .toBe(true);
}

export async function waitForDiffTextAbsent(testPage: Page, text: string, timeout = 5_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((expected) => {
          const containers = Array.from(document.querySelectorAll("diffs-container")).filter(
            (el) => {
              const rect = el.getBoundingClientRect();
              const style = window.getComputedStyle(el);
              return (
                rect.width > 0 &&
                rect.height > 0 &&
                style.display !== "none" &&
                style.visibility !== "hidden"
              );
            },
          );
          return containers.every(
            (container) => !container.shadowRoot?.textContent?.includes(expected),
          );
        }, text),
      { timeout },
    )
    .toBe(true);
}

export async function waitForStoreFileDiffText(
  testPage: Page,
  sessionId: string,
  filePath: string,
  text: string,
  timeout = 30_000,
) {
  await expect
    .poll(
      () =>
        testPage.evaluate(
          ({ sid, path, expected }) => {
            type E2EStoreWindow = Window & {
              __KANDEV_E2E_STORE__?: {
                getState: () => {
                  environmentIdBySessionId: Record<string, string>;
                  gitStatus: {
                    byEnvironmentId: Record<
                      string,
                      { files?: Record<string, { diff?: string }> } | undefined
                    >;
                  };
                };
              };
            };
            const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
            const state = store?.getState();
            if (!state) return false;
            const envKey = state.environmentIdBySessionId[sid] ?? sid;
            return state.gitStatus.byEnvironmentId[envKey]?.files?.[path]?.diff?.includes(expected);
          },
          { sid: sessionId, path: filePath, expected: text },
        ),
      { timeout },
    )
    .toBe(true);
}
