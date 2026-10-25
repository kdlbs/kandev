import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import type { NotificationProvider } from "../../../lib/types/http";

const TURN = "session.turn_finished";
const ASK = "session.clarification_requested";
const PROVIDERS = "/api/v1/notification-providers";

function seedStatus(seedData: SeedData) {
  return {
    head: execFileSync("git", ["-C", seedData.repositoryPath, "rev-parse", "HEAD"], {
      encoding: "utf8",
    }),
    status: execFileSync("git", ["-C", seedData.repositoryPath, "status", "--porcelain=v1"], {
      encoding: "utf8",
    }),
  };
}

type Scenario = {
  testPage: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
};

// @covers AC-PLATFORM-NOTIFICATIONS-001.12
export async function notificationSaveDraftScenario(
  { testPage: page, apiClient, backend, seedData }: Scenario,
  mobile: boolean,
) {
  expect(backend.tmpDir).toContain("kandev");
  expect(seedData.worktreeExecutorProfileId).toBeTruthy();
  const seedBefore = seedStatus(seedData);
  const listed = await apiClient.rawRequest("GET", PROVIDERS);
  expect(listed.ok).toBe(true);
  const { providers } = (await listed.json()) as { providers: NotificationProvider[] };
  const enabled = providers.filter((provider) => provider.enabled);
  let id: string | undefined;
  let release = () => {};
  try {
    for (const provider of enabled) {
      expect(
        (await apiClient.rawRequest("PATCH", `${PROVIDERS}/${provider.id}`, { enabled: false })).ok,
      ).toBe(true);
    }
    const response = await apiClient.rawRequest("POST", PROVIDERS, {
      name: "E2E held notification draft",
      type: "local",
      enabled: false,
      events: [],
    });
    expect(response.ok).toBe(true);
    id = ((await response.json()) as NotificationProvider).id;
    const path = `${PROVIDERS}/${id}`;
    let entered!: () => void;
    const admitted = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let held = false;
    await page.route(`**${path}`, async (route) => {
      if (route.request().method() !== "PATCH" || held) return route.continue();
      held = true;
      const canonical = await route.fetch();
      expect(canonical.ok()).toBe(true);
      expect(route.request().postDataJSON()).toEqual({ events: [TURN] });
      entered();
      await gate;
      await route.fulfill({ response: canonical });
    });
    await page.goto("/settings/preferences/notifications");
    const surface = page.getByTestId(
      mobile ? "notification-events-mobile-list" : "notification-events-desktop-table",
    );
    const turn = surface.getByRole("checkbox", {
      name: "Agent turn finished for E2E held notification draft",
      exact: true,
    });
    const ask = surface.getByRole("checkbox", {
      name: "Agent needs an answer for E2E held notification draft",
      exact: true,
    });
    await expect(surface).toBeVisible();
    const toggle = async (event: string) => {
      if (mobile) {
        const target = surface.getByTestId(`notification-event-toggle-${event}-${id}`);
        await target.scrollIntoViewIfNeeded();
        const box = await target.boundingBox();
        expect(box!.height).toBeGreaterThanOrEqual(44);
        expect(box!.width).toBeGreaterThanOrEqual(44);
        await target.tap();
      } else {
        await (event === TURN ? turn : ask).click();
      }
    };
    await toggle(TURN);
    const save = page.getByRole("button", { name: "Save changes", exact: true });
    await save.click();
    await admitted;
    await expect(ask).toBeEnabled();
    await toggle(ASK);
    const firstResponse = waitForHttp(page, "PATCH", new RegExp(`${path}$`));
    release();
    await firstResponse;
    await expect(save).toBeEnabled();
    await expect(ask).toBeChecked();
    await expect(ask).toHaveAttribute("data-settings-dirty", "true");
    const firstSaved = await apiClient.rawRequest("GET", PROVIDERS);
    expect(
      ((await firstSaved.json()).providers as NotificationProvider[]).find((p) => p.id === id)
        ?.events,
    ).toEqual([TURN]);
    const secondResponse = waitForHttp(page, "PATCH", new RegExp(`${path}$`));
    await save.click();
    expect((await secondResponse).request().postDataJSON()).toEqual({ events: [TURN, ASK] });
    await expect(save).not.toBeVisible();
    await page.reload();
    await expect(ask).toBeChecked();
    await expect(turn).toBeChecked();
    await toggle(TURN);
    const reset = page.getByRole("button", { name: "Reset", exact: true });
    if (mobile) await reset.tap();
    else await reset.click();
    await expect(turn).toBeChecked();
    await expect(ask).toBeChecked();
    await expect(save).not.toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
    if (id) expect((await apiClient.rawRequest("DELETE", `${PROVIDERS}/${id}`)).ok).toBe(true);
    for (const provider of enabled) {
      expect(
        (await apiClient.rawRequest("PATCH", `${PROVIDERS}/${provider.id}`, { enabled: true })).ok,
      ).toBe(true);
    }
    expect(seedStatus(seedData)).toEqual(seedBefore);
  }
}
