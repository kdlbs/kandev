import { test } from "../../fixtures/test-base";
import { expectPluginTabsContained } from "./plugin-tabs-layout";

test("plugin tabs fit their bar at desktop density", async ({ testPage }) => {
  await expectPluginTabsContained(testPage, 28);
});

test("narrow fine-pointer plugin tabs retain touch-sized controls", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  await expectPluginTabsContained(testPage, 44);
});
