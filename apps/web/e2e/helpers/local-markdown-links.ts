import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
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
    const content = `[Open local report](${report})\n\n![Local chart](${encodeURI(image)})\n\n![Missing chart](${dir}/missing.png)\n\n![Broken chart](${dir}/broken.png)`;
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
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    const response = page
      .getByTestId("agent-message-highlight")
      .filter({ hasText: "Open local report" })
      .last();
    const thumbnail = response
      .getByRole("button", { name: "Open Local chart", exact: true })
      .locator("img");
    await expect(thumbnail).toBeVisible();
    await expect
      .poll(() => thumbnail.evaluate((node) => (node as HTMLImageElement).naturalWidth))
      .toBeGreaterThan(0);
    await expect(response.getByRole("status").filter({ hasText: "Missing chart" })).toContainText(
      "Unavailable",
    );
    await expect(response.getByRole("status").filter({ hasText: "Broken chart" })).toContainText(
      "Unavailable",
    );
    await response.getByRole("link", { name: "Open local report", exact: true }).click();
    expect(new URL(page.url()).pathname).toBe(`/t/${task.id}`);
    await expect(page.getByTestId("markdown-preview-toggle").first()).toBeVisible({
      timeout: 10000,
    });
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
