import http from "node:http";
import type { AddressInfo } from "node:net";
import { expect, type FrameLocator, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const PREVIEW_HTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Preview feedback fixture</title>
    <style>
      body { font-family: sans-serif; margin: 24px; min-height: 900px; }
      #save {
        margin-top: 32px;
        padding: 14px 28px;
        background: oklch(0.59 0.2 277);
        color: oklch(0.96 0.02 272);
      }
      #runtime-total { display: block; margin-top: 360px; font-size: 24px; }
      #route-content { padding-top: 24px; }
    </style>
  </head>
  <body>
    <nav><button id="details-route">Open details</button></nav>
    <main id="route-content">
      <h1>Checkout</h1>
      <button id="save" class="primary" aria-label="Save order">Save order</button>
      <span id="runtime-total" role="status" aria-label="Cart total"></span>
    </main>
    <script>
      const total = document.querySelector('#runtime-total');
      total.textContent = 'Generated total $42.00';
      document.querySelector('#details-route').addEventListener('click', () => {
        history.pushState({}, '', '/details?tab=summary');
        document.title = 'Order details';
        document.querySelector('h1').textContent = 'Order details';
      });
    </script>
  </body>
</html>`;

export type PreviewServer = { url: string; close: () => Promise<void> };

export async function startPreviewServer(): Promise<PreviewServer> {
  const server = http.createServer((_request, response) => {
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    response.end(PREVIEW_HTML);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://localhost:${port}`,
    close: () =>
      new Promise<void>((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      ),
  };
}

export async function openBrowserPreview(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  url: string,
): Promise<{ session: SessionPage; frame: FrameLocator }> {
  await apiClient.updateRepository(seedData.repositoryId, { dev_script: "echo preview" });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  await page.getByTestId("dev-server-preview-toggle-center").click();
  await expect(session.browserPanel).toBeVisible();
  await session.browserAddressInput.fill(url);
  await session.browserAddressInput.press("Enter");
  const iframe = session.browserPanel.locator("iframe");
  await expect(iframe).toBeVisible({ timeout: 15_000 });
  const frame = session.browserPanel.frameLocator("iframe");
  await expect(frame.locator("#save")).toBeVisible({ timeout: 15_000 });
  return { session, frame };
}

export async function chooseCapture(page: Page, name: string): Promise<void> {
  const popover = page.getByTestId("preview-feedback-popover");
  if (!(await popover.isVisible())) {
    await page.getByTestId("preview-feedback-trigger").click();
  }
  await expect(popover).toBeVisible();
  const choice = popover.getByRole("button", { name, exact: true });
  await expect(choice).toBeVisible({ timeout: 5_000 });
  const box = await choice.boundingBox();
  const viewport = page.viewportSize();
  if (!box || !viewport || box.y < 0 || box.y + box.height > viewport.height) {
    throw new Error(`capture choice is outside viewport: ${JSON.stringify({ box, viewport })}`);
  }
  await choice.click();
}

export async function waitForScreenshotCaptureMode(frame: FrameLocator): Promise<void> {
  await expect
    .poll(
      () => frame.locator("html").evaluate((element) => (element as HTMLElement).style.cursor),
      {
        timeout: 5_000,
        message: "The preview must enter screenshot-capture mode before the region drag starts",
      },
    )
    .toBe("crosshair");
}

export async function saveDraft(page: Page, comment: string): Promise<void> {
  const draft = page.getByTestId("preview-feedback-draft");
  await expect(draft).toBeVisible({ timeout: 15_000 });
  await draft.getByRole("textbox", { name: "Comment on selection" }).fill(comment);
  await draft.getByRole("button", { name: "Save feedback" }).click();
  await expect(draft).toBeHidden({ timeout: 15_000 });
}

export async function selectGeneratedText(frame: FrameLocator): Promise<void> {
  await frame.locator("#runtime-total").evaluate((element) => {
    const text = element.firstChild;
    if (!text) throw new Error("generated text node missing");
    const value = text.textContent ?? "";
    const start = value.indexOf("$42.00");
    const range = document.createRange();
    range.setStart(text, start);
    range.setEnd(text, start + "$42.00".length);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    element.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  });
}

export async function dragScreenshotRegion(page: Page, target: Locator): Promise<void> {
  const box = await target.boundingBox();
  if (!box) throw new Error("screenshot target did not have a bounding box");
  await page.mouse.move(box.x + 4, box.y + 4);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 4, box.y + box.height - 4);
  await page.mouse.up();
}
