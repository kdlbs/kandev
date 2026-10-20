import type { Locator, Page } from "@playwright/test";
import type { StoreApi } from "zustand";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import type { AppState } from "../../../lib/state/store";
import { captureGatewayRequests } from "../../helpers/archived-session-recovery";
import { waitForHttp } from "../../helpers/causal-waits";

type Context = {
  testPage: Page;
  backend: BackendContext;
  apiClient: ApiClient;
  seedData: SeedData;
};
type StoreWindow = Window & { __KANDEV_E2E_STORE__?: StoreApi<AppState> };
const VIEWS = ["overview", "activity", "workspaces", "providers", "history"];

async function setup(context: Context, enabled: boolean) {
  const { backend, apiClient, seedData } = context;
  const release = await backend.useEnv({
    KANDEV_FEATURES_INTEGRATED_CONTROL: String(enabled),
    KANDEV_FEATURES_COORDINATOR: "true",
  });
  const workspace = await apiClient.createWorkspace("Integrated Beta");
  try {
    const coordinator = await apiClient.createCoordinator(workspace.id, {
      name: "Original Beta coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const opened = await apiClient.rawRequest(
      "POST",
      `/api/v1/workspaces/${workspace.id}/coordinators/${coordinator.id}/conversation`,
      {},
    );
    expect(opened.ok).toBe(true);
    const original = (await opened.json()) as { task_id: string; session_id: string };
    const extra = await apiClient.seedAutomationTaskSession(
      original.task_id,
      "WAITING_FOR_INPUT",
      false,
    );
    return {
      workspace,
      coordinator,
      original,
      extraSessionId: extra.id,
      dispose: async () => {
        await apiClient.deleteWorkspace(workspace.id, workspace.name);
        await release();
      },
    };
  } catch (error) {
    await apiClient.deleteWorkspace(workspace.id, workspace.name);
    await release();
    throw error;
  }
}

async function navigation(page: Page, mobile: boolean): Promise<Locator> {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").click();
    await expect(page.getByTestId("app-nav-sheet")).toBeVisible();
    return page.getByTestId("app-nav-sheet").getByTestId("integrated-control-navigation");
  }
  return page.getByTestId("app-sidebar").getByTestId("integrated-control-navigation");
}

async function openPicker(page: Page, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").click();
    await page.getByTestId("app-nav-sheet").getByTestId("mobile-workspace-trigger").click();
  } else await page.getByTestId("sidebar-workspace-trigger").click();
}

async function assertTarget(target: Locator, touch: boolean, pointer = true) {
  await target.scrollIntoViewIfNeeded();
  const box = await target.boundingBox();
  expect(box).not.toBeNull();
  if (touch) expect(box!.height).toBeGreaterThanOrEqual(44);
  if (pointer)
    expect(await target.evaluate((element) => getComputedStyle(element).cursor)).toBe("pointer");
  expect(
    await target.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const hit = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return hit !== null && element.contains(hit);
    }),
  ).toBe(true);
}

function observe(page: Page) {
  const gateway = captureGatewayRequests(page);
  const writes: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (
      (request.method() !== "GET" && /\/conversation(?:\?|$)/.test(url.pathname)) ||
      /capabilities.*discover|discover.*capabilities|\/api\/v1\/agents\/available$/.test(
        url.pathname,
      )
    )
      writes.push(request.method() + " " + url.pathname);
  });
  return {
    begin: () => {
      writes.length = 0;
      gateway.length = 0;
    },
    assertNoExecution: async () => {
      const refreshed = waitForHttp(page, "GET", /\/api\/v1\/workspaces(?:\?|$)/);
      await page.evaluate(() => window.dispatchEvent(new Event("focus")));
      await refreshed;
      await expect(
        page
          .getByTestId("integrated-control-page")
          .getByRole("button", { name: "Refresh", exact: true }),
      ).toBeEnabled();
      expect(writes).toEqual([]);
      expect(
        gateway.filter((frame) =>
          /session\.(launch|ensure|fork|recover|restore|resume|prompt)|task\.(prepare|resume|start)|agent\.capabilities/.test(
            frame.action ?? "",
          ),
        ),
      ).toEqual([]);
    },
  };
}

async function storeSelection(page: Page) {
  return page.evaluate(() => {
    const state = (window as StoreWindow).__KANDEV_E2E_STORE__!.getState();
    return {
      activeWorkspaceId: state.workspaces.activeId,
      activeTaskId: state.tasks.activeTaskId,
      activeSessionId: state.tasks.activeSessionId,
      prs: state.taskPRs,
    };
  });
}

async function originalConversation(context: Context, mobile: boolean) {
  const { testPage: page } = context;
  const seeded = await setup(context, true);
  const observation = observe(page);
  try {
    await page.goto(`/kanban?workspace=${context.seedData.workspaceId}`);
    await openPicker(page, mobile);
    const before = await storeSelection(page);
    observation.begin();
    await page.getByTestId("workspace-picker-integrated-control").click();
    await expect(page.getByTestId("control-dashboard-overview")).toBeVisible();
    for (const view of VIEWS) {
      const nav = await navigation(page, mobile);
      const link = nav.getByTestId(`control-nav-${view}`);
      await assertTarget(link, mobile);
      await link.click();
      if (mobile) await expect(page.getByTestId("app-nav-sheet")).toHaveCount(0);
      await expect(page.getByTestId(`control-dashboard-${view}`)).toBeVisible();
    }
    const nav = await navigation(page, mobile);
    await nav.getByTestId(`control-conversation-${seeded.coordinator.id}`).click();
    const conversation = page.getByTestId("integrated-control-conversation");
    await expect(conversation).toHaveAttribute("data-task-id", seeded.original.task_id);
    await expect(conversation).toHaveAttribute("data-workspace-id", seeded.workspace.id);
    const selector = conversation.getByRole("combobox");
    await assertTarget(selector, mobile, false);
    await selector.selectOption(seeded.extraSessionId);
    await expect(conversation).toHaveAttribute("data-session-id", seeded.extraSessionId);
    const editor = conversation.getByTestId("chat-input-editor");
    await expect(editor).toHaveAttribute("contenteditable", "true");
    await editor.fill("Original Beta session draft");
    await expect(editor).toContainText("Original Beta session draft");
    const backNav = await navigation(page, mobile);
    await backNav.getByTestId("control-nav-overview").click();
    const returnNav = await navigation(page, mobile);
    await returnNav.getByTestId(`control-conversation-${seeded.coordinator.id}`).click();
    await expect(conversation).toHaveAttribute("data-session-id", seeded.extraSessionId);
    await expect(conversation.getByTestId("chat-input-editor")).toContainText(
      "Original Beta session draft",
    );
    expect(await storeSelection(page)).toEqual(before);
    await page.goBack();
    await expect(page.getByTestId("control-dashboard-overview")).toBeVisible();
    await observation.assertNoExecution();
  } finally {
    await seeded.dispose();
  }
}

async function refreshCollections(page: Page, path: string) {
  const loaded = waitForHttp(page, "GET", new RegExp(`${path}$`));
  const button = page
    .getByTestId("integrated-control-page")
    .getByRole("button", { name: "Refresh", exact: true });
  await button.click();
  await loaded;
  await expect(button).toBeEnabled();
}

async function unavailableCollections(context: Context, mobile: boolean) {
  const seeded = await setup(context, true);
  const page = context.testPage;
  const observation = observe(page);
  try {
    const path = `/api/v1/workspaces/${seeded.workspace.id}/coordinators`;
    await page.route(`**${path}`, (route) =>
      route.fulfill({ status: 403, json: { error: "denied" } }),
    );
    const ordinaryCatalogReady = waitForHttp(page, "GET", /\/api\/v1\/agents\/available$/);
    await page.goto(`/kanban?workspace=${context.seedData.workspaceId}`);
    await ordinaryCatalogReady;
    await openPicker(page, mobile);
    observation.begin();
    await page.getByTestId("workspace-picker-integrated-control").click();
    await expect(page.getByTestId("control-dashboard-overview")).toBeVisible();
    const errorNav = await navigation(page, mobile);
    await errorNav.getByTestId("control-nav-workspaces").click();
    await expect(page.getByTestId("control-dashboard-workspaces").getByRole("alert")).toContainText(
      ["Could not load this collection.", "Some workspace data could not be loaded."],
    );
    await page.unroute(`**${path}`);
    await refreshCollections(page, path);
    const restoredNav = await navigation(page, mobile);
    await restoredNav.getByTestId(`control-conversation-${seeded.coordinator.id}`).click();
    await expect(page.getByTestId("integrated-control-conversation")).toHaveAttribute(
      "data-task-id",
      seeded.original.task_id,
    );
    await page.route(`**${path}`, (route) => route.fulfill({ json: { coordinators: [] } }));
    await refreshCollections(page, path);
    await expect(
      page.getByText("This conversation is unavailable or no longer accessible.", { exact: true }),
    ).toBeVisible();
    await expect(page.getByTestId("integrated-control-conversation")).toHaveCount(0);
    await page.unroute(`**${path}`);
    const missingNav = await navigation(page, mobile);
    await missingNav.getByTestId("control-nav-overview").click();
    await refreshCollections(page, path);
    await page.route(`**/api/v1/tasks/${seeded.original.task_id}`, (route) =>
      route.fulfill({ status: 404, json: { error: "missing" } }),
    );
    const taskNav = await navigation(page, mobile);
    await taskNav.getByTestId(`control-conversation-${seeded.coordinator.id}`).click();
    await expect(page.getByRole("alert")).toContainText(
      "This conversation is unavailable or no longer accessible.",
    );
    await observation.assertNoExecution();
  } finally {
    await seeded.dispose();
  }
}

export function controlNavigationCases(mobile: boolean) {
  test("OFF retains ordinary navigation and does not read another workspace's coordinators", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    const context = { testPage, backend, apiClient, seedData };
    const seeded = await setup(context, false);
    const reads: string[] = [];
    context.testPage.on("request", (request) => {
      if (request.url().includes(`/workspaces/${seeded.workspace.id}/coordinators`))
        reads.push(request.url());
    });
    try {
      await context.testPage.goto(`/control?workspace=${context.seedData.workspaceId}`);
      await expect(context.testPage.getByTestId("integrated-control-page")).toHaveCount(0);
      await openPicker(context.testPage, mobile);
      await expect(context.testPage.getByTestId("workspace-picker-integrated-control")).toHaveCount(
        0,
      );
      expect(reads).toEqual([]);
    } finally {
      await seeded.dispose();
    }
  });

  test("opt-in navigation preserves the second workspace's original conversation, draft and selected runtime without execution", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    const context = { testPage, backend, apiClient, seedData };
    await originalConversation(context, mobile);
  });

  test("denied, failed and missing collections stay visible and never create a replacement", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    await unavailableCollections({ testPage, backend, apiClient, seedData }, mobile);
  });
}

export function controlPhoneGeometryCase() {
  test("phone navigation has one contained scroll owner, usable targets and keyboard dismissal", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    const context = { testPage, backend, apiClient, seedData };
    const seeded = await setup(context, true);
    const page = context.testPage;
    try {
      for (let index = 0; index < 4; index++) {
        await context.apiClient.createCoordinator(seeded.workspace.id, {
          name: `Additional coordinator ${index}`,
          agent_profile_id: context.seedData.agentProfileId,
          executor_profile_id: context.seedData.worktreeExecutorProfileId,
          task_agent_profile_id: context.seedData.agentProfileId,
          task_executor_profile_id: context.seedData.worktreeExecutorProfileId,
        });
      }
      await page.goto("/control/overview");
      const nav = await navigation(page, true);
      await assertTarget(nav.getByTestId("control-nav-history"), true);
      await assertTarget(nav.getByRole("button", { name: "Refresh", exact: true }), true);
      await assertTarget(nav.getByText(seeded.workspace.name, { exact: true }), true);
      await assertTarget(nav.getByText(seeded.coordinator.name, { exact: true }), true);
      await assertTarget(nav.getByTestId(`control-conversation-${seeded.coordinator.id}`), true);
      const sheet = page.getByTestId("app-nav-sheet");
      await sheet.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) =>
              Number.isFinite(animation.effect?.getComputedTiming().iterations),
            )
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      const box = await sheet.boundingBox();
      const viewport = page.viewportSize()!;
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.y).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
      expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
      const owners = await sheet.evaluate(
        (element) =>
          [...element.querySelectorAll("nav, [data-testid=integrated-control-navigation]")].filter(
            (child) => getComputedStyle(child).overflowY === "auto",
          ).length,
      );
      expect(owners).toBe(1);
      const scroll = await sheet.locator("nav").evaluate((element) => {
        element.scrollTop = 0;
        element.scrollTop = element.scrollHeight;
        return {
          top: element.scrollTop,
          overflow: element.scrollHeight - element.clientHeight,
          documentTop: document.documentElement.scrollTop,
        };
      });
      expect(scroll.overflow).toBeGreaterThan(0);
      expect(scroll.top).toBeGreaterThan(0);
      expect(scroll.documentTop).toBe(0);
      await page.keyboard.press("Escape");
      await expect(sheet).toHaveCount(0);
      await expect(page.getByTestId("app-nav-trigger")).toBeFocused();
    } finally {
      await seeded.dispose();
    }
  });
}
