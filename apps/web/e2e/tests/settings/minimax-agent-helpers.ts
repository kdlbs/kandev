import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

export async function exerciseMiniMaxSetup(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  info: TestInfo,
  capture: PrAssetCapture,
) {
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
  fs.writeFileSync(path.join(bin, "mcode"), `#!${process.execPath}\n${source}`, { mode: 0o755 });
  const release = await backend.useEnv({
    KANDEV_MOCK_AGENT: "true",
    PATH: `${bin}${path.delimiter}${process.env.PATH}`,
  });
  try {
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
    const help = page.getByText(/For a Global account, press Ctrl\+C/);
    await expect(help).toBeVisible();
    await expect.poll(() => fs.existsSync(path.join(root, "authenticated"))).toBe(true);
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
      caption: "MiniMax subscription login and region guidance",
    });
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
    const { agents } = await api.listAgents();
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
      await capture.screenshot("profile", {
        caption: "MiniMax native model saved in an agent profile",
      });
    } finally {
      await api.deleteAgentProfile(profile.id, true);
    }
  } finally {
    await release();
    fs.rmSync(root, { recursive: true, force: true });
  }
}
