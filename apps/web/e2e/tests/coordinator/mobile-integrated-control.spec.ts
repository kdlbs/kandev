import { test } from "../../fixtures/test-base";
import { controlNavigationCases, controlPhoneGeometryCase } from "./integrated-control-helpers";

test.describe("Integrated control phone", () => {
  controlNavigationCases(true);
  controlPhoneGeometryCase();
});
