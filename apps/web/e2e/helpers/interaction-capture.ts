import type { Page } from "@playwright/test";
import type { PrAssetCapture } from "./pr-asset-capture";

/** Keep host shell identity out of synthetic workbench screenshots. */
export async function captureWorkbenchInteraction(
  page: Page,
  capture: PrAssetCapture,
  name: string,
  caption: string,
) {
  if (!capture.capturing) return;
  const redaction = await page.addStyleTag({
    content: ".xterm-screen { visibility: hidden !important; }",
  });
  try {
    await capture.screenshot(name, { caption });
  } finally {
    await redaction.evaluate((element) => element.remove());
  }
}
