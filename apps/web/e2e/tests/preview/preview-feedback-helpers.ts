import http from "node:http";
import type { AddressInfo } from "node:net";
import { expect, type FrameLocator, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const PREVIEW_HTML = `<!doctype html>
<html>
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
  <body>
    <nav><button id="details-route">Open details</button></nav>
    <main id="route-content">
      <h1>Checkout</h1>
      <button id="save" class="primary" aria-label="Save order">Save order</button>
      <canvas id="gl-canvas" width="160" height="80" style="display:block;margin-top:16px"></canvas>
      <span id="runtime-total" role="status" aria-label="Cart total"></span>
      <div id="scroller" style="height:160px;overflow:auto;margin-top:24px;border:1px solid #ccc">
        <div id="scroll-target" style="height:60px;background:#fde68a">Scrollable panel</div>
        <div style="height:600px"></div>
      </div>
      <section id="centered" style="width:240px;height:80px;margin:24px auto;background:#e0e7ff">
        Centered card
      </section>
      <section id="fluid" style="width:60%;height:80px;margin-top:24px;background:#dcfce7">
        Fluid card
      </section>
    </main>
    <script>
      // WebGL draws once without preserveDrawingBuffer, like Flutter's CanvasKit.
      const gl = document.querySelector('#gl-canvas').getContext('webgl');
      window.__kandevFixtureWebGL = Boolean(gl);
      if (gl) {
        gl.clearColor(0.85, 0.1, 0.1, 1);
        gl.clear(gl.COLOR_BUFFER_BIT);
        gl.enable(gl.SCISSOR_TEST);
        gl.scissor(0, 0, 80, 80);
        gl.clearColor(0.1, 0.2, 0.85, 1);
        gl.clear(gl.COLOR_BUFFER_BIT);
      }
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

export const COMMENT_PANEL_STORAGE_KEY = "kandev:preview-comment-panel-open";

/**
 * The comment panel overlays narrow previews; flows that click under it start
 * with it hidden. Call before the page loads.
 */
export async function startWithCommentPanelHidden(page: Page): Promise<void> {
  await page.addInitScript(
    (key) => window.localStorage.setItem(key, "false"),
    COMMENT_PANEL_STORAGE_KEY,
  );
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

const CAPTURE_TOOLS = {
  "Select text": { tool: "text", mode: "text" },
  "Select element": { tool: "element", mode: "element" },
  "Select screenshot region": { tool: "region", mode: "screenshot" },
} as const;

/** Enters annotation mode if needed and selects a tool on the preview toolbar. */
export async function chooseCapture(
  page: Page,
  name: string,
  _frame?: FrameLocator,
): Promise<void> {
  const choice = CAPTURE_TOOLS[name as keyof typeof CAPTURE_TOOLS];
  if (!choice) throw new Error(`unsupported preview capture choice: ${name}`);
  const trigger = page.getByTestId("preview-feedback-trigger");
  await expect(async () => {
    if ((await trigger.getAttribute("data-capture-mode")) === choice.mode) return;
    if ((await trigger.getAttribute("data-annotation-active")) !== "true") await trigger.click();
    await page.getByTestId(`preview-tool-${choice.tool}`).click({ timeout: 1_000 });
    await expect(trigger).toHaveAttribute("data-capture-mode", choice.mode, { timeout: 1_000 });
  }).toPass({ timeout: 12_000, intervals: [250, 500, 1_000] });
}

/** Switches the toolbar to Interact so page input passes through. */
export async function chooseInteract(page: Page): Promise<void> {
  const trigger = page.getByTestId("preview-feedback-trigger");
  await page.getByTestId("preview-tool-interact").click();
  await expect(trigger).toHaveAttribute("data-capture-mode", "none");
}

/** Leaves annotation mode with the toolbar's Exit button. */
export async function exitAnnotation(page: Page): Promise<void> {
  await page.getByTestId("preview-annotation-exit").click();
  await expect(page.getByTestId("preview-feedback-trigger")).toHaveAttribute(
    "data-annotation-active",
    "false",
  );
}

/** Wait until the iframe has applied screenshot mode before dispatching a drag. */
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

/** The shared selection comment surface holding the current capture draft. */
export function draftSurface(page: Page): Locator {
  return page.getByTestId("preview-feedback-draft");
}

export async function fillDraftComment(page: Page, comment: string): Promise<void> {
  await page.getByTestId("preview-feedback-draft-input").fill(comment);
}

export async function submitDraft(page: Page): Promise<void> {
  await page.getByTestId("preview-feedback-draft-save").click();
}

export async function saveDraft(page: Page, comment: string): Promise<void> {
  const draft = draftSurface(page);
  await expect(draft).toBeVisible({ timeout: 15_000 });
  await fillDraftComment(page, comment);
  await submitDraft(page);
  await expect(draft).toBeHidden({ timeout: 15_000 });
}

/** Selects all of an element's text in the page, as a user's drag would. */
export async function selectText(frame: FrameLocator, selector: string): Promise<void> {
  await frame.locator(selector).evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
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

/** Region outlines drawn over the preview, in creation order. */
export function regionOutlines(page: Page): Locator {
  return page.getByTestId("preview-region");
}

export async function dragScreenshotRegion(
  page: Page,
  frame: FrameLocator,
  target: Locator,
): Promise<void> {
  // The inspector changes the iframe's cursor only after it receives the
  // capture-mode message. Wait for that state before sending pointer events.
  await expect(frame.locator("html")).toHaveCSS("cursor", "crosshair");
  const box = await target.boundingBox();
  if (!box) throw new Error("screenshot target did not have a bounding box");
  await page.mouse.move(box.x + 4, box.y + 4);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 4, box.y + box.height - 4);
  await page.mouse.up();
}
