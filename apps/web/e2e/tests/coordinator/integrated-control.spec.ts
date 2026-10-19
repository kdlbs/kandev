import { test } from "../../fixtures/test-base";
import { controlNavigationCases, controlPhoneGeometryCase } from "./integrated-control-helpers";

test.describe("Integrated control desktop", () => controlNavigationCases(false));
test.describe("Integrated control narrow fine pointer", () => {
  test.beforeEach(async ({ testPage }) => {
    await testPage.setViewportSize({ width: 765, height: 720 });
  });
  controlPhoneGeometryCase();
});
