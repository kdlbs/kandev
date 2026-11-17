import { expect, type Page } from "@playwright/test";
import { test } from "../fixtures/test-base";
import { dwell } from "./causal-waits";
import { assertVerticalCentersAligned } from "./layout-assertions";
import { assertDeliveryRecoveryGeometry } from "./delivery-recovery-controls";
import { verifyNativeResumeClearsRecoveryNotice } from "./native-resume-notice";

// The backend integration test controls real SQL projection. This gate tests the
// browser's pending-request and failure states without adding a production API.
async function holdResumeRequest(page: Page) {
  let requestCount = 0;
  let release: ((error?: string) => void) | undefined;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    server.onMessage((message) => socket.send(message));
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part) as { type?: string; action?: string; id?: string };
        if (frame.type !== "request" || frame.action !== "session.recover") {
          server.send(part);
          continue;
        }
        requestCount++;
        release = (error) => {
          if (error) {
            socket.send(
              JSON.stringify({
                ...frame,
                type: "error",
                payload: { code: "E2E_PROJECTION_TIMEOUT", message: error },
              }),
            );
          } else server.send(part);
        };
      }
    });
  });
  return {
    count: () => requestCount,
    release: (error?: string) => {
      const pending = release;
      release = undefined;
      pending?.(error);
    },
  };
}

export function registerNativeResumeProjectionTests() {
  // @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
  // @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.26
  for (const timeout of [false, true]) {
    test(`Resume stays busy during recovery${timeout ? " and permits retry after timeout" : " and succeeds once"}`, async ({
      testPage,
      apiClient,
      seedData,
      backend,
      prCapture,
    }) => {
      const gate = await holdResumeRequest(testPage);
      await verifyNativeResumeClearsRecoveryNotice(testPage, apiClient, seedData, backend.tmpDir, {
        legacy: "readiness",
        afterResumeClick: async () => {
          try {
            const banner = testPage.getByTestId("failed-session-banner");
            const status = banner.getByTestId("recovery-progress-status");
            const resume = testPage.getByTestId("recovery-resume-button");
            const retryConnection = testPage.getByTestId("recovery-retry-connection-button");
            const stop = testPage.getByTestId("recovery-stop-button");
            await expect.poll(gate.count).toBe(1);
            await expect(status).toBeVisible();
            await expect(status).toHaveText("Resuming session");
            await expect(status).toHaveAttribute("aria-live", "polite");
            await expect(
              banner.getByRole("status").filter({ hasText: "Resuming session" }),
            ).toHaveCount(1);
            await expect(resume).toBeDisabled();
            await expect(retryConnection).toBeDisabled();
            await expect(stop).toBeEnabled();
            const statusPrecedesActions = await status.evaluate((element) => {
              const action = element.parentElement?.querySelector(
                '[data-testid="recovery-resume-button"]',
              );
              return Boolean(
                action &&
                element.compareDocumentPosition(action) & Node.DOCUMENT_POSITION_FOLLOWING,
              );
            });
            expect(statusPrecedesActions).toBe(true);
            await assertPendingResumeGeometry(testPage, !timeout);
            await retryConnection.evaluate((button) => (button as HTMLButtonElement).click());
            await resume.evaluate((button) => (button as HTMLButtonElement).click());
            await dwell(
              testPage,
              300,
              "negative-assertion",
              "Disabled duplicate recovery actions must not send another session.recover request",
            );
            expect(gate.count()).toBe(1);
            await prCapture.screenshot(`${test.info().project.name}-resume-waiting`, {
              caption: "Resume remains busy while recovery evidence is checked.",
            });
            if (timeout) {
              gate.release("Native resume projection is incomplete. Deadline exceeded.");
              await expect(
                banner.getByRole("status").filter({ hasText: "Failed to resume session" }),
              ).toBeVisible();
              await expect(resume).toBeEnabled();
              await expect(testPage.locator(".tiptap.ProseMirror:visible")).toHaveCount(0);
              if (test.info().project.name === "mobile-chrome") await resume.tap();
              else await resume.click();
              await expect.poll(gate.count).toBe(2);
              await expect(resume).toBeDisabled();
            }
          } finally {
            gate.release();
          }
        },
      });
    });
  }
}

async function assertPendingResumeGeometry(page: Page, checkBreakpoints = true) {
  const originalViewport = page.viewportSize();
  if (!originalViewport) throw new Error("Resume projection requires a fixed viewport");
  const status = page.getByTestId("recovery-progress-status");
  const spinner = status.locator(".spinner-grid");
  const label = status.locator(":scope > span").nth(1);
  const readStatusStyles = () =>
    status.evaluate((element) => {
      const spinnerElement = element.querySelector<HTMLElement>(".spinner-grid");
      if (!spinnerElement) throw new Error("Recovery progress spinner is missing");
      const statusStyle = getComputedStyle(element);
      return {
        fontSize: statusStyle.fontSize,
        fontWeight: statusStyle.fontWeight,
        color: statusStyle.color,
        spinnerColor: getComputedStyle(spinnerElement).color,
      };
    });
  const expectedStyles = await readStatusStyles();
  const touch = await page.evaluate(() => matchMedia("(pointer: coarse)").matches);
  expect(expectedStyles.fontSize).toBe("14px");
  expect(expectedStyles.fontWeight).toBe("500");
  expect(expectedStyles.spinnerColor).not.toBe("");

  try {
    const widths = checkBreakpoints
      ? new Set([originalViewport.width, 767, 768])
      : [originalViewport.width];
    for (const width of widths) {
      await page.setViewportSize({ width, height: originalViewport.height });
      await assertDeliveryRecoveryGeometry(page, "recovery-resume-button", {
        allowWrappedActions: touch && width >= 768,
      });
      await assertVerticalCentersAligned(spinner, label, `Resume status at ${width}px`);
      expect(await readStatusStyles()).toEqual(expectedStyles);
    }
  } finally {
    await page.setViewportSize(originalViewport);
  }
}
