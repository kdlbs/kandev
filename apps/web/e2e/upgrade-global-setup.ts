import fs from "node:fs";
import path from "node:path";
import { assertUpgradeCI } from "../../../scripts/upgrade/ci-only.mjs";

export default function setup() {
  assertUpgradeCI();
  for (const file of [
    "../backend/bin/kandev",
    "../backend/bin/agentctl",
    "../backend/bin/mock-agent",
    "dist/index.html",
  ]) {
    if (!fs.existsSync(path.resolve(file)))
      throw new Error(
        `upgrade candidate artifact missing: ${file}; CI must build candidate artifacts before rehearsal`,
      );
  }
}
