import { test } from "../../fixtures/test-base";
import { expectPluginTabsContained } from "./plugin-tabs-layout";

test("phone plugin tabs fit their bar with equal touch targets", async ({ testPage }) => {
  await expectPluginTabsContained(testPage, 44);
});

test("wide touch-screen plugin tabs keep touch target heights", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 1024, height: 768 });
  await expectPluginTabsContained(testPage, 44);
});
