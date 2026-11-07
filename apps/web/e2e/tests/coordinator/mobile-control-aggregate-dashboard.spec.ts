import { test } from "../../fixtures/office-fixture";
import { controlAggregateDashboardCases } from "./control-aggregate-dashboard-helpers";

test.describe("Integrated control Office aggregate phone", () =>
  controlAggregateDashboardCases(true));
