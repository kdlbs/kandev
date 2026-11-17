import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { watchWs } from "./causal-waits";
import { waitForAgentMessage } from "./session";
import { SessionPage } from "../pages/session-page";

export async function verifyLocalMarkdownLinks(page: Page, api: ApiClient, seed: SeedData) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-local-links-"));
  const report = path.join(dir, "report.md");
  const image = path.join(dir, "chart one.png");
  fs.writeFileSync(report, "# External report marker\n\nOriginal local file.\n");
  fs.writeFileSync(
    image,
    Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
      "base64",
    ),
  );
  fs.writeFileSync(path.join(dir, "broken.png"), fs.readFileSync(image).subarray(0, 16));
  try {
    const content = `[Open local report](${report})\n\n[![Local chart](${encodeURI(image)} "Chart source")](${report})\n\n![Missing chart](${dir}/missing.png)\n\n![Broken chart](${dir}/broken.png)`;
    const script = `e2e:message(${JSON.stringify(content)})`;
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Local Markdown links",
      seed.agentProfileId,
      {
        description: script,
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Missing session");
    await waitForAgentMessage(api, task.session_id, "Open local report");
    const gateway = watchWs(page);
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    const response = page
      .getByTestId("agent-message-highlight")
      .filter({ hasText: "Open local report" })
      .last();
    const imageButton = response.getByRole("button", { name: "Open Local chart", exact: true });
    const thumbnail = imageButton.locator("img");
    await expect(thumbnail).toBeVisible();
    await expect(thumbnail).toHaveAttribute("title", "Chart source");
    await expect
      .poll(() => thumbnail.evaluate((node) => (node as HTMLImageElement).naturalWidth))
      .toBeGreaterThan(0);
    await imageButton.click();
    const imageDialog = page.getByRole("dialog").last();
    await expect(imageDialog).toBeVisible();
    const fullImage = imageDialog.getByRole("img");
    await expect(fullImage).toBeVisible();
    await expect
      .poll(() => fullImage.evaluate((node) => (node as HTMLImageElement).naturalWidth))
      .toBeGreaterThan(0);
    expect(new URL(page.url()).pathname).toBe(`/t/${task.id}`);
    await expect(page.getByTestId("markdown-preview-toggle")).toHaveCount(0);
    await page.evaluate(async () => {
      const finiteAnimations = document.getAnimations().filter((animation) => {
        const iterations = animation.effect?.getComputedTiming().iterations;
        return typeof iterations === "number" && Number.isFinite(iterations);
      });
      await Promise.all(
        finiteAnimations.map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    const viewport = page.viewportSize();
    const dialogBounds = await imageDialog.boundingBox();
    expect(viewport).not.toBeNull();
    expect(dialogBounds).not.toBeNull();
    expect(dialogBounds!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBounds!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBounds!.x + dialogBounds!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dialogBounds!.y + dialogBounds!.height).toBeLessThanOrEqual(viewport!.height);
    if (page.viewportSize()!.width < 768) {
      await expect(page.getByTestId("mobile-file-viewer-panel")).toHaveCount(0);
    }
    await page.keyboard.press("Escape");
    await expect(imageDialog).toBeHidden();
    await expect(response.getByRole("status").filter({ hasText: "Missing chart" })).toContainText(
      "Unavailable",
    );
    await expect(response.getByRole("status").filter({ hasText: "Broken chart" })).toContainText(
      "Unavailable",
    );
    const reportRead = gateway.waitForResponse("workspace.file.get", {
      requestWhere: (payload) => String(payload.path ?? "").endsWith("report.md"),
    });
    await response.getByRole("link", { name: "Open local report", exact: true }).click();
    await reportRead;
    expect(new URL(page.url()).pathname).toBe(`/t/${task.id}`);
    await expect(page.getByTestId("markdown-preview-toggle").first()).toBeVisible();
    await page.getByTestId("markdown-preview-toggle").first().click();
    await expect(page.getByRole("heading", { name: "External report marker" })).toBeVisible();
    if (page.viewportSize()!.width < 768) {
      await expect(page.getByTestId("mobile-file-viewer-panel")).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    }
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}
