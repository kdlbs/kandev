import { expect, type Page } from "@playwright/test";
import { test } from "../fixtures/test-base";
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
  return { count: () => requestCount, release: (error?: string) => release?.(error) };
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
          const banner = testPage.getByTestId("failed-session-banner");
          const resume = testPage.getByTestId("recovery-resume-button");
          await expect.poll(gate.count).toBe(1);
          await expect(banner.getByRole("status").filter({ hasText: "Resuming..." })).toBeVisible();
          await expect(resume).toBeDisabled();
          await resume.evaluate((button) => (button as HTMLButtonElement).click());
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
          gate.release();
        },
      });
    });
  }
}
