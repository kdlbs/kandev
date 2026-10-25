import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import { LinearSettingsPage } from "../../pages/linear-settings-page";

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
function initRepository(repository: string) {
  execFileSync("git", ["init", "-b", "main", repository]);
  writeFileSync(path.join(repository, "fixture.txt"), "Disposable Linear connection fixture\n");
  execFileSync("git", ["-C", repository, "add", "fixture.txt"]);
  execFileSync("git", [
    "-C",
    repository,
    "-c",
    "user.name=Linear Fixture",
    "-c",
    "user.email=linear@example.test",
    "commit",
    "-m",
    "test: seed disposable Linear repository",
  ]);
}

function mutationGate(method: string) {
  return {
    method,
    captured: Promise.withResolvers<void>(),
    release: Promise.withResolvers<void>(),
  };
}

async function confirmLinearRemoval(page: Page, settings: LinearSettingsPage, touch: boolean) {
  if (touch) await settings.deleteButton.tap();
  else await settings.deleteButton.click();
  const confirm = page.getByTestId("linear-remove-confirm");
  if (touch) {
    await expect(page.getByRole("dialog")).toHaveAttribute("data-slot", "drawer-content");
    await confirm.tap();
  } else await confirm.click();
}

// @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4
export async function exerciseLinearRefresh({
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
  const repository = mkdtempSync(path.join(runtimeDirectory, "linear-refresh-"));
  let repositoryBefore: ReturnType<typeof snapshot> | undefined;
  const name = `Linear refresh ${testInfo.testId}`;
  let workspace: { id: string } | undefined;
  let executor: { id: string } | undefined;
  let repositoryId: string | undefined;
  const release = Promise.withResolvers<void>();
  const captured = Promise.withResolvers<void>();
  const held: Promise<void>[] = [];
  let holding = true;
  let heldMutation: ReturnType<typeof mutationGate> | null = null;
  const mutations: ReturnType<typeof mutationGate>[] = [];
  const errors: unknown[] = [];
  try {
    initRepository(repository);
    repositoryBefore = snapshot(repository);
    workspace = await api.createWorkspace(name);
    executor = await api.createExecutor(name, "worktree");
    await api.updateWorkspace(workspace.id, { default_executor_id: executor.id });
    repositoryId = (await api.createRepository(workspace.id, repository)).id;
    if (!touch) {
      await api.setLinearConfig({ workspaceId: workspace.id, secret: "fixture-only-key" });
      await api.waitForIntegrationAuthHealthy("linear", { workspaceId: workspace.id });
    }
    await page.clock.install();
    const settings = new LinearSettingsPage(page);
    await settings.gotoWorkspace(workspace.id);
    await expect(settings.secretInput).toBeEnabled();
    await expect(settings.secretInput).toHaveAttribute(
      "placeholder",
      touch ? "lin_api_..." : "••••••••",
    );
    const workspaceId = workspace.id;
    await page.route("**/api/v1/linear/config?workspace_id=*", (route) => {
      const gate = heldMutation;
      if (
        gate &&
        route.request().method() === gate.method &&
        new URL(route.request().url()).searchParams.get("workspace_id") === workspaceId
      ) {
        const pending = (async () => {
          try {
            const response = await route.fetch();
            expect(response.status()).toBe(200);
            gate.captured.resolve();
            await gate.release.promise;
            await route.fulfill({ response });
          } catch (error) {
            gate.captured.reject(error);
            throw error;
          }
        })();
        held.push(pending);
        return pending;
      }
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
    if (!touch) {
      await api.mockLinearSetAuthResult({ ok: false, error: "Saved Linear probe failure" });
      await api.setLinearConfig({ workspaceId, secret: "fixture-only-key" });
      await expect
        .poll(async () => {
          const response = await api.rawRequest(
            "GET",
            `/api/v1/linear/config?workspace_id=${workspaceId}`,
          );
          return response.json();
        })
        .toMatchObject({ lastOk: false, lastError: "Saved Linear probe failure" });
    }
    await settings.secretInput.fill("fixture-saved-key");
    const saved = waitForHttp(page, "POST", /\/api\/v1\/linear\/config$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === workspaceId,
    });
    if (touch) await settings.saveButton.tap();
    else await settings.saveButton.click();
    const ack = await saved;
    expect(ack.status()).toBe(200);
    await ack.finished();
    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.secretInput).toHaveAttribute("data-settings-dirty", "false");
    if (!touch) await expect(settings.statusBanner).toContainText("Saved Linear probe failure");
    const stale = waitForHttp(page, "GET", /\/api\/v1\/linear\/config$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === workspaceId,
    });
    holding = false;
    release.resolve();
    await Promise.all(held);
    const old = await stale;
    expect(old.status()).toBe(touch ? 204 : 200);
    if (old.status() !== 204) await old.finished();
    await dwell(
      page,
      200,
      "negative-assertion",
      "Rejected Linear read has no publication event; observe settled stale transport cannot replace accepted metadata.",
    );
    await expect(settings.deleteButton).toBeVisible();
    await expect(settings.secretInput).toHaveAttribute("placeholder", "••••••••");
    if (!touch) await expect(settings.statusBanner).toContainText("Saved Linear probe failure");
    await expect(settings.saveButton).toHaveCount(0);
    await settings.secretInput.fill("reset-draft");
    const reset = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset", exact: true });
    if (touch) await reset.tap();
    else await reset.click();
    await expect(settings.secretInput).toHaveValue("");
    await settings.secretInput.fill("next-saved-key");
    const pendingSave = mutationGate("POST");
    heldMutation = pendingSave;
    mutations.push(pendingSave);
    const next = waitForHttp(page, "POST", /\/api\/v1\/linear\/config$/);
    if (touch) await settings.saveButton.tap();
    else await settings.saveButton.click();
    await pendingSave.captured.promise;
    await expect(settings.deleteButton).toBeDisabled();
    await expect(page.getByTestId("linear-test-button")).toBeDisabled();
    pendingSave.release.resolve();
    expect((await next).status()).toBe(200);
    heldMutation = null;
    await expect(settings.saveButton).toHaveCount(0);
    await page.reload();
    await expect(settings.secretInput).toHaveAttribute("placeholder", "••••••••");
    await expect(settings.deleteButton).toBeVisible();
    await settings.secretInput.scrollIntoViewIfNeeded();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    const screenshot = testInfo.outputPath("linear-refresh-saved.png");
    await page.screenshot({ path: screenshot });
    await testInfo.attach("linear-refresh-saved", { path: screenshot, contentType: "image/png" });
    const pendingRemove = mutationGate("DELETE");
    heldMutation = pendingRemove;
    mutations.push(pendingRemove);
    const removed = waitForHttp(page, "DELETE", /\/api\/v1\/linear\/config$/);
    await confirmLinearRemoval(page, settings, touch);
    await pendingRemove.captured.promise;
    if (touch) await expect(page.getByRole("dialog")).toHaveCount(0);
    await settings.secretInput.fill("blocked while removing");
    await expect(settings.saveButton).toBeDisabled();
    await expect(page.getByTestId("linear-test-button")).toBeDisabled();
    pendingRemove.release.resolve();
    expect((await removed).status()).toBe(200);
    heldMutation = null;
    await expect(settings.deleteButton).toHaveCount(0);
    await expect(settings.secretInput).toHaveAttribute("placeholder", "lin_api_...");
    await expect(settings.secretInput).toHaveValue("blocked while removing");
    await expect(settings.secretInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(settings.saveButton).toBeEnabled();
  } catch (error) {
    errors.push(new Error("Primary Linear refresh scenario failed", { cause: error }));
  }
  const clean = async (label: string, action: () => unknown) => {
    try {
      await action();
    } catch (error) {
      errors.push(new Error(`Cleanup: ${label}`, { cause: error }));
    }
  };
  holding = false;
  release.resolve();
  for (const gate of mutations) gate.release.resolve();
  await clean("held routes", () => Promise.all(held));
  await clean("routes and navigation", async () => {
    if (!page.isClosed()) {
      await page.unrouteAll({ behavior: "wait" });
      await page.goto("/settings");
    }
  });
  await clean("Linear configuration", async () => {
    if (workspace)
      expect(
        (await api.rawRequest("DELETE", `/api/v1/linear/config?workspace_id=${workspace.id}`)).ok,
      ).toBe(true);
  });
  await clean("repository row", async () => {
    if (repositoryId) await api.deleteRepository(repositoryId);
  });
  await clean("workspace", async () => {
    if (workspace) await api.deleteWorkspace(workspace.id, name);
  });
  await clean("executor", async () => {
    if (executor) await api.deleteExecutor(executor.id);
  });
  await clean("seed immutability", () => expect(snapshot(seedRepository)).toEqual(seedBefore));
  await clean("source immutability", () => expect(snapshot(sourceRoot)).toEqual(sourceBefore));
  await clean("owned repository immutability", () => {
    if (repositoryBefore) expect(snapshot(repository)).toEqual(repositoryBefore);
  });
  await clean("owned repository directory", () =>
    rmSync(repository, { recursive: true, force: true }),
  );
  if (errors.length) throw new AggregateError(errors, "Linear scenario and cleanup outcomes");
}
