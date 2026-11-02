import { test, expect } from "../../fixtures/upgrade-backend";
import { seedUpgrade, interruptUpgrade } from "../../helpers/upgrade-seed";

import {
  assertHistoricalMessagesPreserved,
  assertPassiveUpgradeReads,
  assertPreserved,
  captureUpgradeState,
  saveUpgradeSnapshot,
} from "../../helpers/upgrade-oracle";
import { checkUpgradeDatabase } from "../../helpers/upgrade-database";

import {
  assertUpgradeLifecycleResult,
  browserUpgradeFollowup,
  exerciseUpgradeLifecycle,
} from "../../helpers/upgrade-lifecycle";

// @covers AC-PLATFORM-UPGRADE-REGRESSION-001.1, AC-PLATFORM-UPGRADE-REGRESSION-001.2
test("baseline seed provenance", async ({ upgrade }) => {
  const seed = await seedUpgrade(upgrade);
  expect(seed.tasks.waiting.session_id).toBeTruthy();
  expect((await upgrade.api.listTaskSessions(seed.tasks.waiting.id)).sessions).toHaveLength(2);
});

// @covers AC-PLATFORM-UPGRADE-REGRESSION-002.1, AC-PLATFORM-UPGRADE-REGRESSION-002.2, AC-PLATFORM-UPGRADE-REGRESSION-002.3, AC-PLATFORM-UPGRADE-REGRESSION-002.4
test("preserves release data and replays startup", async ({ upgrade }) => {
  const seed = await seedUpgrade(upgrade);
  const before = await interruptUpgrade(upgrade, seed);
  await upgrade.start("candidate");
  const after = await captureUpgradeState(upgrade, seed);
  saveUpgradeSnapshot(upgrade, "candidate-state.json", after);
  assertPreserved(before, after);
  await upgrade.stop();
  checkUpgradeDatabase(upgrade, seed, true);
  await upgrade.start("candidate");
  assertPreserved(before, await captureUpgradeState(upgrade, seed));
  await upgrade.stop();
  checkUpgradeDatabase(upgrade, seed, false);
});

// @covers AC-PLATFORM-UPGRADE-REGRESSION-003.1, AC-PLATFORM-UPGRADE-REGRESSION-003.2, AC-PLATFORM-UPGRADE-REGRESSION-003.3, AC-PLATFORM-UPGRADE-REGRESSION-003.4
test("executes old and new task lifecycles after upgrade", async ({ upgrade, page }) => {
  const seed = await seedUpgrade(upgrade);
  const before = await interruptUpgrade(upgrade, seed);
  await upgrade.start("candidate");
  assertPreserved(before, await captureUpgradeState(upgrade, seed));
  await assertPassiveUpgradeReads(upgrade, seed);
  await exerciseUpgradeLifecycle(upgrade, seed);
  if (upgrade.engine === "sqlite") await browserUpgradeFollowup(page, upgrade, seed);
  const changed = await captureUpgradeState(upgrade, seed);
  assertHistoricalMessagesPreserved(before, changed);
  saveUpgradeSnapshot(upgrade, "post-action.json", changed);
  await upgrade.stop();
  checkUpgradeDatabase(upgrade, seed, true);
  await upgrade.start("candidate");
  assertPreserved(changed, await captureUpgradeState(upgrade, seed));
  await assertUpgradeLifecycleResult(upgrade, seed);
  await upgrade.stop();
  checkUpgradeDatabase(upgrade, seed, false);
});
