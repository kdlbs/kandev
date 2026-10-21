import { test } from "../../fixtures/upgrade-backend";
import { seedUpgrade, interruptUpgrade } from "../../helpers/upgrade-seed";
import {
  assertHistoricalMessagesPreserved,
  assertPassiveUpgradeReads,
  assertPreserved,
  captureUpgradeState,
} from "../../helpers/upgrade-oracle";
import { browserUpgradeFollowup, resumeUpgradeTask } from "../../helpers/upgrade-lifecycle";

test("phone preserves release history and sends a follow-up after upgrade", async ({
  upgrade,
  page,
}) => {
  if (upgrade.engine !== "sqlite") throw new Error("phone rehearsal requires SQLite");
  const seed = await seedUpgrade(upgrade);
  const before = await interruptUpgrade(upgrade, seed);
  await upgrade.start("candidate");
  assertPreserved(before, await captureUpgradeState(upgrade, seed));
  await assertPassiveUpgradeReads(upgrade, seed);
  await resumeUpgradeTask(upgrade, seed.tasks.waiting, seed.profileId, seed.executorProfileId);
  await browserUpgradeFollowup(page, upgrade, seed);
  const after = await captureUpgradeState(upgrade, seed);
  assertHistoricalMessagesPreserved(before, after);
  await upgrade.start("candidate");
  assertPreserved(after, await captureUpgradeState(upgrade, seed));
});
