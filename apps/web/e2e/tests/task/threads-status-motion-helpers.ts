import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";
import { captureThreadSettings, seedThreadPresentation } from "./threads-presentation-helpers";

export function threadsStatusMotionScenarios() {
  test("preserves Threads status through reduced-motion changes", async ({
    testPage: page,
    apiClient: api,
    seedData: seed,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const original = await captureThreadSettings(api);
    await page.emulateMedia({ reducedMotion: "reduce" });
    const finish = path.join(backend.tmpDir, `thread-motion-${randomUUID()}`);
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Threads status motion",
      seed.agentProfileId,
      {
        description: `e2e:message("Thread motion ready")\ne2e:await_file(${JSON.stringify(finish)})`,
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Expected a session for Threads motion");
    try {
      await waitForAgentMessage(api, task.session_id, "Thread motion ready");
      await seedThreadPresentation(api, { layout: "grid", maxChats: 1 });
      await page.goto(`/threads?workspace=${seed.workspaceId}`);
      const column = page.getByTestId(`thread-column-${task.id}`);
      const icon = column.getByTestId("thread-status-working");
      await expect(icon).toBeVisible();
      const label = await icon.getAttribute("aria-label");
      const running = () =>
        icon.evaluate(
          (element) =>
            element.getAnimations().filter((animation) => animation.playState === "running").length,
        );
      await expect.poll(running).toBe(0);
      await expect(icon).toHaveCSS("width", "14px");
      await expect(icon).toHaveCSS("height", "14px");
      await prCapture.screenshot(`${test.info().project.name}-threads-reduced-motion`, {
        caption: "Threads retains its working status and icon geometry under reduced motion.",
      });
      await page.emulateMedia({ reducedMotion: "no-preference" });
      await expect.poll(running).toBe(1);
      expect(
        await icon.evaluate((element) => {
          const animation = element.getAnimations()[0];
          return {
            engine: animation.constructor.name,
            duration: animation.effect?.getTiming().duration,
            iterations: String(animation.effect?.getTiming().iterations),
          };
        }),
      ).toEqual({ engine: "CSSAnimation", duration: 1000, iterations: "Infinity" });
      await icon.evaluate((element) => {
        element.parentElement!.style.transform = "translateX(200vw)";
      });
      await expect.poll(running).toBe(0);
      await expect(icon).toHaveCSS("animation-play-state", "paused");
      await page.emulateMedia({ reducedMotion: "reduce" });
      await expect.poll(running).toBe(0);
      await icon.evaluate((element) => {
        element.parentElement!.style.removeProperty("transform");
      });
      await expect(icon).toBeVisible();
      await expect(icon).toHaveAttribute("aria-label", label!);
      await page.emulateMedia({ reducedMotion: "no-preference" });
      await expect.poll(running).toBe(1);
      await column.evaluate((element) => {
        element.style.transform = "translateY(200vh)";
      });
      await expect(column).not.toBeInViewport();
      await expect.poll(running).toBe(0);
      await expect(icon).toHaveCSS("animation-play-state", "paused");
      await column.evaluate((element) => element.style.removeProperty("transform"));
      await expect(column).toBeInViewport();
      await expect.poll(running).toBe(1);
      fs.writeFileSync(finish, "finish");
      await waitForSessionDone(
        api,
        task.id,
        task.session_id,
        "Threads motion turn settled",
        30_000,
      );
      await expect(icon).toHaveCount(0);
    } finally {
      fs.writeFileSync(finish, "finish");
      await api.saveUserSettings(original);
    }
  });
}
