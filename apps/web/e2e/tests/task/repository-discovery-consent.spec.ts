import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { KanbanPage } from "../../pages/kanban-page";
import {
  DISCOVERY_FAILURE_ROOT,
  installRepositoryDiscoveryFailureRoute,
} from "./repository-discovery-failure-helpers";
import fs from "node:fs";
import path from "node:path";

useRegularMode();

test.describe("Desktop repository discovery consent", () => {
  test("confirms Home directly without opening a folder picker", async ({
    testPage,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "true" });

    let confirmationRequests = 0;
    let confirmationBody: string | null = null;
    let directoryListingRequests = 0;
    await testPage.addInitScript(() => {
      const win = window as typeof window & {
        __TAURI_INTERNALS__?: {
          invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
          transformCallback: () => number;
        };
      };
      const commands: string[] = [];
      Object.defineProperty(win, "__TAURI_INTERNALS__", {
        configurable: true,
        value: {
          transformCallback: () => 1,
          invoke: async (command: string) => {
            commands.push(command);
            if (command === "plugin:event|listen") return 1;
            if (command === "get_native_notification_permission") return "granted";
            if (command === "request_native_notification_permission") return "granted";
            if (command === "get_update_state") {
              return {
                phase: "idle",
                currentVersion: "",
                latestVersion: null,
                releaseNotes: null,
                releaseUrl: null,
                checkedAtEpochMs: null,
                downloadedBytes: null,
                totalBytes: null,
                installSupported: false,
                installUnsupportedReason: null,
                error: null,
              };
            }
            return null;
          },
        },
      });
      Object.defineProperty(win, "__kandevFolderPickerCommands", {
        configurable: true,
        value: commands,
      });
    });
    await testPage.route("**/api/v1/workspaces/*/repositories/discovery**", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          json: {
            roots: [],
            repositories: [],
            total: 0,
            desktop_runtime: true,
            root_states: [],
            home_confirmation_required: confirmationRequests === 0,
          },
        });
        return;
      }
      await route.continue();
    });
    await testPage.route("**/api/v1/repositories/discovery/roots/confirm-home", async (route) => {
      confirmationRequests += 1;
      confirmationBody = route.request().postData();
      await route.fulfill({
        json: {
          id: "confirmed-home",
          path: "/desktop-user-home",
          display_path: "~",
          state: "connected",
        },
      });
    });
    testPage.on("request", (request) => {
      if (request.url().includes("/api/v1/fs/list-dir") && request.method() === "GET") {
        directoryListingRequests += 1;
      }
    });

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
      await testPage.getByRole("button", { name: "Add Local Repository" }).click();
      const controls = testPage
        .getByRole("dialog", { name: "Add Local Repository" })
        .getByTestId("discovery-root-controls");
      const continueHome = controls.getByRole("button", { name: "Continue Home Discovery" });
      await expect(continueHome).toBeVisible();
      await continueHome.click();

      await expect.poll(() => confirmationRequests).toBe(1);
      await expect(continueHome).toBeHidden();
      expect(confirmationBody).toBeNull();
      expect(directoryListingRequests).toBe(0);
      expect(
        await testPage.evaluate(
          () =>
            (window as typeof window & { __kandevFolderPickerCommands?: string[] })
              .__kandevFolderPickerCommands ?? [],
        ),
      ).not.toContain("pick_directory");
    } finally {
      await backend.restart();
    }
  });

  test("shows compact discovery actions only inside the repository selector", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(120_000);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "true" });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("discovery-root-controls")).toHaveCount(0);

    await dialog.getByTestId("repo-chip-trigger").first().click();
    const controls = testPage.getByTestId("discovery-root-controls");
    const chooseFolders = controls.getByTestId("folder-picker-trigger");
    const refreshRepositories = controls.getByRole("button", {
      name: "Refresh repositories",
    });
    await expect(controls).toBeVisible();
    await waitForFiniteAnimations(controls);

    const [chooseBox, refreshBox] = await Promise.all([
      chooseFolders.boundingBox(),
      refreshRepositories.boundingBox(),
    ]);
    expect(chooseBox).not.toBeNull();
    expect(refreshBox).not.toBeNull();
    expect(Math.abs(refreshBox!.height - chooseBox!.height)).toBeLessThanOrEqual(1);
    expect(refreshBox!.height).toBeLessThanOrEqual(32);
  });

  test("keeps failed-root diagnostics out of the server selector", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(120_000);
    await installRepositoryDiscoveryFailureRoute(testPage);
    await backend.restart({ KANDEV_DESKTOP_RUNTIME: "false" });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("repo-chip-trigger").first().click();

    await expect(testPage.getByTestId("discovery-root-controls")).toHaveCount(0);
    await expect(testPage.getByTestId("discovery-failure")).toHaveCount(0);
    await expect(testPage.getByText(DISCOVERY_FAILURE_ROOT, { exact: true })).toHaveCount(0);
    const availableRepository = testPage.getByRole("option", { name: /healthy-project/ });
    await expect(availableRepository).toBeEnabled();
    await availableRepository.click();
    await expect(dialog.getByTestId("repo-chip-trigger").first()).toContainText("healthy-project");

    await dialog.getByTestId("repo-chip-trigger").first().click();
    const refreshResponse = testPage.waitForResponse(
      (response) =>
        /\/api\/v1\/workspaces\/[^/]+\/repositories$/.test(new URL(response.url()).pathname) &&
        response.request().method() === "GET" &&
        response.ok(),
    );
    await testPage.getByTestId("repo-refresh-button").click();
    await refreshResponse;
    await expect(testPage.getByTestId("discovery-failure")).toHaveCount(0);
    await expect(testPage.getByRole("option", { name: /healthy-project/ })).toBeVisible();
  });

  test("uses the native picker and keeps the discovery root recoverable", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const selectedRoot = fs.mkdtempSync(path.join(backend.tmpDir, "desktop-discovery-root-"));
    let rootSaved = false;
    let directoryListingRequests = 0;
    const onRequest = (request: { url(): string; method(): string }) => {
      if (request.url().includes("/api/v1/fs/list-dir") && request.method() === "GET") {
        directoryListingRequests += 1;
      }
    };

    try {
      await backend.restart({ KANDEV_DESKTOP_RUNTIME: "true" });
      await testPage.addInitScript(
        ({ selectedPath }: { selectedPath: string }) => {
          const win = window as typeof window & {
            __TAURI_INTERNALS__?: {
              invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
              transformCallback: () => number;
            };
            __kandevFolderPickerCommands?: string[];
          };
          const commands: string[] = [];
          win.__kandevFolderPickerCommands = commands;
          Object.defineProperty(win, "__TAURI_INTERNALS__", {
            configurable: true,
            value: {
              transformCallback: () => 1,
              invoke: async (command: string) => {
                commands.push(command);
                if (command === "pick_directory") {
                  return { status: "selected", path: selectedPath };
                }
                if (command === "plugin:event|listen") return 1;
                if (command === "get_native_notification_permission") return "granted";
                if (command === "request_native_notification_permission") return "granted";
                if (command === "get_update_state") {
                  return {
                    phase: "idle",
                    currentVersion: "",
                    latestVersion: null,
                    releaseNotes: null,
                    releaseUrl: null,
                    checkedAtEpochMs: null,
                    downloadedBytes: null,
                    totalBytes: null,
                    installSupported: false,
                    installUnsupportedReason: null,
                    error: null,
                  };
                }
                return null;
              },
            },
          });
        },
        { selectedPath: selectedRoot },
      );
      testPage.on("request", onRequest);

      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
      await testPage.getByRole("button", { name: "Add Local Repository" }).click();
      const dialog = testPage.getByRole("dialog", { name: "Add Local Repository" });
      const controls = dialog.getByTestId("discovery-root-controls");
      await expect(controls).toBeVisible();

      const addResponse = testPage.waitForResponse(
        (response) =>
          response.url().endsWith("/api/v1/repositories/discovery/roots") &&
          response.request().method() === "POST" &&
          response.ok(),
      );
      await controls
        .getByRole("button", { name: "Choose folders to discover repositories" })
        .click();
      expect((await addResponse).status()).toBe(201);
      rootSaved = true;

      await expect(controls.getByTitle(selectedRoot)).toBeVisible();
      expect(
        await testPage.evaluate(
          () =>
            (window as typeof window & { __kandevFolderPickerCommands?: string[] })
              .__kandevFolderPickerCommands ?? [],
        ),
      ).toContain("pick_directory");
      expect(directoryListingRequests).toBe(0);

      const refreshResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/workspaces/") &&
          response.url().includes("/repositories/discovery/refresh") &&
          response.request().method() === "POST" &&
          response.ok(),
      );
      await controls.getByRole("button", { name: "Refresh repositories" }).click();
      await refreshResponse;

      const removeResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/repositories/discovery/roots?") &&
          response.request().method() === "DELETE" &&
          response.ok(),
      );
      await controls.getByRole("button", { name: "Remove" }).click();
      await removeResponse;
      rootSaved = false;
      await expect(controls.getByTitle(selectedRoot)).toHaveCount(0);
    } finally {
      testPage.off("request", onRequest);
      if (rootSaved) {
        await apiClient
          .rawRequest(
            "DELETE",
            `/api/v1/repositories/discovery/roots?path=${encodeURIComponent(selectedRoot)}`,
          )
          .catch(() => undefined);
      }
      fs.rmSync(selectedRoot, { recursive: true, force: true });
      await backend.restart();
    }
  });
});
