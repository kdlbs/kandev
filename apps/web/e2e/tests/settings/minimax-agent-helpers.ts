import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

export async function exerciseMiniMaxSetup({
  page,
  backend,
  api,
  info,
  capture,
  seed,
}: {
  page: Page;
  backend: BackendContext;
  api: ApiClient;
  info: TestInfo;
  capture: PrAssetCapture;
  seed: SeedData;
}) {
  const activate = (control: Locator) =>
    info.project.name === "mobile-chrome"
      ? control.tap({ timeout: 10_000 })
      : control.click({ timeout: 10_000 });
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "minimax-fixture-"));
  const bin = path.join(root, "bin");
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(root, "package.json"), JSON.stringify({ type: "module" }));
  const source = fs.readFileSync(
    path.resolve(__dirname, "../../helpers/minimax-acp-fixture.mjs"),
    "utf8",
  );
  fs.writeFileSync(path.join(root, "pending-mcode"), `#!${process.execPath}\n${source}`);
  fs.writeFileSync(path.join(bin, "npm"), `#!${process.execPath}\n${source}`, { mode: 0o755 });
  const release = await backend.useEnv({
    KANDEV_E2E_MOCK: "false",
    KANDEV_MOCK_AGENT: "false",
    KANDEV_MOCK_PROVIDERS: "true",
    PATH: `${bin}${path.delimiter}${process.env.PATH}`,
  });
  let createdAgent = false;
  try {
    await page
      .context()
      .addCookies([{ name: "kandev_locale", value: "pt-pt", url: backend.baseUrl }]);
    await page.goto("/settings/agents/browse");
    const installCard = page.getByTestId("install-card-minimax-acp");
    await expect(installCard).toBeVisible({ timeout: 15_000 });
    await expect(installCard).toContainText(
      "MiniMax Code com o seu servidor ACP nativo e autenticação por subscrição.",
    );
    await page.context().addCookies([{ name: "kandev_locale", value: "en", url: backend.baseUrl }]);
    await page.reload();
    await expect(installCard.locator("code")).toContainText("@minimax-ai/code@0.5.10");
    expect(
      await installCard.evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    await installCard.scrollIntoViewIfNeeded();
    await capture.screenshot("install", { caption: "MiniMax native CLI installation" });
    await activate(page.getByTestId("install-button-minimax-acp"));
    await expect.poll(() => fs.existsSync(path.join(root, "installed-args.json"))).toBe(true);
    await expect
      .poll(
        async () => {
          const response = await fetch(`${backend.baseUrl}/api/v1/agents/available`);
          const { agents } = await response.json();
          const miniMax = agents.find((agent: { name: string }) => agent.name === "minimax-acp");
          if (miniMax?.model_config.status === "failed")
            throw new Error(JSON.stringify(miniMax.model_config));
          return miniMax?.model_config.status;
        },
        { timeout: 30_000 },
      )
      .toBe("auth_required");
    await page.goto("/settings/agents");
    const auth = page.getByTestId("auth-icon-minimax-acp");
    await expect(auth).toBeVisible({ timeout: 15_000 });
    await activate(auth);
    const region = info.project.name === "mobile-chrome" ? "Global" : "Mainland China";
    const choice = page.getByRole("button", { name: region, exact: true });
    await expect(choice).toBeVisible();
    if (info.project.name === "chromium") {
      const desktopSize = page.viewportSize()!;
      await page.setViewportSize({ width: 390, height: 844 });
      await expect(page.getByTestId("minimax-login-region-drawer")).toBeVisible();
      await expect
        .poll(async () => (await choice.boundingBox())!.height)
        .toBeGreaterThanOrEqual(44);
      await page.setViewportSize(desktopSize);
      await expect(page.getByTestId("minimax-login-region-drawer")).not.toBeVisible();
      await expect(choice).toBeVisible();
    }
    if (info.project.name === "mobile-chrome") {
      expect((await choice.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await expect(page.getByTestId("minimax-login-region-drawer")).toBeVisible();
    }
    expect(fs.existsSync(path.join(root, "authenticated"))).toBe(false);
    await page.keyboard.press("Escape");
    await expect(choice).not.toBeVisible();
    await activate(auth);
    await expect(choice).toBeVisible();
    expect(fs.existsSync(path.join(root, "authenticated"))).toBe(false);
    if (capture.capturing) {
      await page.getByRole("dialog").evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter(
              (animation) =>
                animation.playState === "running" &&
                Number.isFinite(animation.effect?.getComputedTiming().endTime),
            )
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
    }
    await capture.screenshot("region", {
      caption: "Choose the MiniMax subscription account region",
    });
    const started = page.waitForResponse(
      (response) =>
        response.url().endsWith("/agent-login/agents/minimax-acp/start") &&
        response.request().method() === "POST",
    );
    await activate(choice);
    const login = await (await started).json();
    const nativeRegion = info.project.name === "mobile-chrome" ? "global" : "cn";
    const same = await api.rawRequest("POST", "/api/v1/agent-login/agents/minimax-acp/start", {
      command_variant: nativeRegion,
    });
    expect(same.status).toBe(200);
    expect((await same.json()).session_id).toBe(login.session_id);
    const conflict = await api.rawRequest("POST", "/api/v1/agent-login/agents/minimax-acp/start", {
      command_variant: nativeRegion === "cn" ? "global" : "cn",
    });
    expect(conflict.status).toBe(409);
    expect((await conflict.json()).error_code).toBe("login_command_conflict");
    const live = await api.rawRequest(
      "GET",
      `/api/v1/agent-login/sessions/${login.session_id}/status`,
    );
    expect(await live.json()).toMatchObject({
      session_id: login.session_id,
      running: true,
      cmd: login.cmd,
    });
    const command = page.getByTestId("agent-login-command").locator("code");
    await expect(command).toHaveCSS("white-space", "pre-wrap");
    expect(await command.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const help = page.getByText(/MiniMax stores credentials in/);
    await expect(help).toBeVisible();
    await expect.poll(() => fs.existsSync(path.join(root, "authenticated"))).toBe(true);
    expect(JSON.parse(fs.readFileSync(path.join(root, "login-args.json"), "utf8"))).toEqual([
      "login",
      "--no-browser",
      "--region",
      nativeRegion,
    ]);
    await expect(command).toContainText(`--region ${nativeRegion}`);
    const dialog = page.getByRole("dialog");
    await dialog.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .filter(
            (animation) =>
              animation.playState === "running" &&
              Number.isFinite(animation.effect?.getComputedTiming().endTime),
          )
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const bounds = await dialog.boundingBox();
    const viewport = page.viewportSize()!;
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height + 1);
    await capture.screenshot("login", {
      caption: "MiniMax subscription login for the selected account region",
    });
    await exerciseConflictingLogin({ page, api, info, region: nativeRegion, original: login });
    const done = page.getByRole("button", { name: "Done", exact: true });
    await expect(done).toBeVisible();
    await activate(done);
    await expect
      .poll(
        async () => {
          const response = await fetch(`${backend.baseUrl}/api/v1/agents/available`);
          const { agents } = await response.json();
          const miniMax = agents.find((agent: { name: string }) => agent.name === "minimax-acp");
          if (miniMax?.model_config.status === "failed")
            throw new Error(JSON.stringify(miniMax.model_config));
          return miniMax?.model_config.status;
        },
        { timeout: 30_000 },
      )
      .toBe("ok");
    let { agents } = await api.listAgents();
    if (!agents.some((item) => item.name === "minimax-acp")) {
      const created = await api.rawRequest("POST", "/api/v1/agents", {
        name: "minimax-acp",
        profiles: [],
      });
      expect(created.ok, await created.text()).toBe(true);
      createdAgent = true;
      ({ agents } = await api.listAgents());
    }
    const agent = agents.find((item) => item.name === "minimax-acp");
    expect(agent).toBeTruthy();
    const profile = await api.createAgentProfile(agent!.id, "MiniMax fixture profile", {
      model: "m:minimax:MiniMax-M3:v:thinking",
    });
    try {
      await page.goto(`/settings/agents/minimax-acp/profiles/${profile.id}`);
      const selector = page.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toContainText("MiniMax-M3", { timeout: 15_000 });
      await activate(selector);
      await activate(page.getByRole("option", { name: "MiniMax-M2.7-highspeed", exact: true }));
      await expect(selector).toContainText("MiniMax-M2.7-highspeed");
      await activate(page.getByText("Profile name", { exact: true }));
      await expect(
        page.getByRole("option", { name: "MiniMax-M2.7-highspeed", exact: true }),
      ).not.toBeVisible();
      const save = page.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(save).toBeEnabled();
      await activate(save);
      await expect
        .poll(async () => (await api.getAgentProfile(profile.id)).model)
        .toBe("m:minimax:MiniMax-M2.7-highspeed:v:thinking");
      await page.reload();
      await expect(selector).toContainText("MiniMax-M2.7-highspeed", { timeout: 15_000 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
      await page
        .context()
        .addCookies([{ name: "kandev_locale", value: "pt-pt", url: backend.baseUrl }]);
      await page.reload();
      const passthrough = page.getByTestId("cli-passthrough-toggle");
      await expect(passthrough).toContainText("Modo terminal da CLI");
      await expect(passthrough).toContainText(
        "Mostrar o terminal diretamente em vez da interface de conversa",
      );
      await page
        .context()
        .addCookies([{ name: "kandev_locale", value: "en", url: backend.baseUrl }]);
      await page.reload();
      await expect(selector).toContainText("MiniMax-M2.7-highspeed");
      await capture.screenshot("profile", {
        caption: "MiniMax native model saved in an agent profile",
      });
      const task = await api.createTaskWithAgent(
        seed.workspaceId,
        "Native MiniMax fixture task",
        profile.id,
        {
          description: "Reply with the fixture response.",
          workflow_id: seed.workflowId,
          workflow_step_id: seed.startStepId,
          repository_ids: [seed.repositoryId],
        },
      );
      expect(task.session_id).toBeTruthy();
      await waitForAgentMessage(api, task.session_id!, "MiniMax fixture response", 30_000);
      await waitForSessionDone(
        api,
        task.id,
        task.session_id!,
        "MiniMax fixture turn settles",
        30_000,
      );
      const turns = fs
        .readFileSync(path.join(root, "turns.jsonl"), "utf8")
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line));
      expect(turns).toContainEqual({
        args: ["acp"],
        model: "m:minimax:MiniMax-M2.7-highspeed:v:thinking",
      });
      await api.deleteTask(task.id);
    } finally {
      await api.deleteAgentProfile(profile.id, true);
    }
  } finally {
    if (createdAgent) await api.deleteCustomAgentByName("minimax-acp");
    await release();
    fs.rmSync(root, { recursive: true, force: true });
  }
}

async function exerciseConflictingLogin({
  page,
  api,
  info,
  region,
  original,
}: {
  page: Page;
  api: ApiClient;
  info: TestInfo;
  region: string;
  original: { session_id: string; cmd: string[] };
}) {
  const other = await page.context().newPage();
  const activate = (control: Locator) =>
    info.project.name === "mobile-chrome" ? control.tap() : control.click();
  try {
    await other.goto("/settings/agents");
    await activate(other.getByTestId("auth-icon-minimax-acp"));
    await activate(
      other.getByRole("button", {
        name: region === "cn" ? "Global" : "Mainland China",
        exact: true,
      }),
    );
    await expect(
      other.getByText(
        "Another sign-in command is already running. Close that terminal before trying again.",
      ),
    ).toBeVisible();
    const dialog = other.getByRole("dialog");
    await dialog.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .filter(
            (animation) =>
              animation.playState === "running" &&
              Number.isFinite(animation.effect?.getComputedTiming().endTime),
          )
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    const capture = new PrAssetCapture(other, info.file, { captureKey: "conflict" });
    await capture.screenshot("login", {
      caption: "Competing region sign-in preserves the original terminal",
    });
    capture.flush();
    await activate(other.getByRole("button", { name: "Close", exact: true }));
    await expect(dialog).not.toBeVisible();
    const live = await api.rawRequest(
      "GET",
      `/api/v1/agent-login/sessions/${original.session_id}/status`,
    );
    expect(await live.json()).toMatchObject({
      session_id: original.session_id,
      running: true,
      cmd: original.cmd,
    });
  } finally {
    await other.close();
  }
}
