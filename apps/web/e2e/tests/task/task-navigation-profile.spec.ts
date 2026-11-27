import { test, expect } from "../../fixtures/test-base";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import {
  exposeNavigationStore,
  selectNavigationTask,
  showNavigationFiles,
} from "./task-navigation-helpers";
import {
  measureSelection,
  prepareProfileTask,
  profileMachine,
  sampleMemory,
  saveProfileArtifact,
  seedProfile,
} from "./task-navigation-profile-helpers";

test.skip(process.env.KANDEV_NAVIGATION_PROFILE !== "1", "opt-in navigation measurement");

for (const debug of [false, true]) {
  test(`profile repeated task navigation with debug=${debug}`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(240_000);
    const restoreEnv = await backend.useEnv({
      KANDEV_DEBUG_DEV_MODE: String(debug),
      KANDEV_DEBUG_PPROF_ENABLED: String(debug),
      KANDEV_LOG_LEVEL: "info",
    });
    try {
      const tasks = await seedProfile(apiClient, seedData, backend);
      const gate = await routeNavigationResponses(testPage);
      await exposeNavigationStore(testPage);
      const a = await prepareProfileTask(testPage, apiClient, tasks[0], backend, true);
      const b = await prepareProfileTask(testPage, apiClient, tasks[1], backend, false);
      const bootDebug = await testPage.evaluate(
        () => (window as Window & { __KANDEV_DEBUG?: boolean }).__KANDEV_DEBUG === true,
      );
      await testPage.evaluate((value) => {
        (window as Window & { __KANDEV_DEBUG?: boolean }).__KANDEV_DEBUG = value;
      }, debug);
      await measureSelection(testPage, gate, a);
      await measureSelection(testPage, gate, b);
      const cdp = await testPage.context().newCDPSession(testPage);
      await cdp.send("Performance.enable");
      const before = await sampleMemory(testPage, cdp, backend, testInfo, "before");
      const performanceBefore = await cdp.send("Performance.getMetrics");
      await testPage.evaluate(() => {
        const win = window as Window & { __navigationLongTasks?: number[] };
        win.__navigationLongTasks = [];
        new PerformanceObserver((list) => {
          for (const entry of list.getEntries()) win.__navigationLongTasks!.push(entry.duration);
        }).observe({ type: "longtask" });
      });
      const selections = [];
      for (let i = 0; i < 10; i++)
        for (const item of [a, b]) selections.push(await measureSelection(testPage, gate, item));
      const performanceAfter = await cdp.send("Performance.getMetrics");
      const longTasks = await testPage.evaluate(
        () => (window as Window & { __navigationLongTasks?: number[] }).__navigationLongTasks,
      );
      const after = await sampleMemory(testPage, cdp, backend, testInfo, "after");
      // Capture CPU attribution separately from normal timing samples.
      await cdp.send("Profiler.enable");
      await cdp.send("Profiler.start");
      await selectNavigationTask(testPage, a.task.title);
      await showNavigationFiles(testPage, false);
      await expect(
        testPage.locator(`[data-testid="file-tree-node"][data-path="${a.marker}"]:visible`),
      ).toBeVisible();
      const trace = await cdp.send("Profiler.stop");
      await saveProfileArtifact(testInfo, "navigation-cpu-profile", JSON.stringify(trace.profile));
      await saveProfileArtifact(
        testInfo,
        "navigation-profile",
        JSON.stringify(
          {
            machine: profileMachine(),
            browser: await testPage.context().browser()!.version(),
            debug,
            bootDebug,
            fixture: {
              tasks: 32,
              folders: 21,
              baseTreeFiles: 630,
              seededMessagesPerActiveSession: 200,
              actualMessageCounts: [a.messageCount, b.messageCount],
              changedFiles: [a.changedFiles, b.changedFiles],
            },
            timingDefinition:
              "Driver action start to URL/DOM visibility and two requestAnimationFrame callbacks; paint opportunity is not a compositor measurement. No CPU profiler runs in timed selections.",
            before,
            after,
            performanceBefore,
            performanceAfter,
            longTasks,
            selections,
          },
          null,
          2,
        ),
      );
      await cdp.detach();
    } finally {
      await restoreEnv();
    }
  });
}
