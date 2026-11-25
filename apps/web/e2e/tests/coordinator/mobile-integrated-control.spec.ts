import { test } from "../../fixtures/test-base";
import {
  controlNavigationCases,
  controlPhoneGeometryCase,
  controlCircuitCases,
} from "./integrated-control-helpers";

test.describe("Integrated control phone", () => {
  controlNavigationCases(true);
  controlCircuitCases();
  controlPhoneGeometryCase();
});
