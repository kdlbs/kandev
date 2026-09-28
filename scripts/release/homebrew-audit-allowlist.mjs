#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const COMPACT_KANDEV_EXCEPTION =
  "libexec/bin/agentctl-{darwin,linux}-{amd64,arm64}";

export function removeCompactKandevAuditException(filePath) {
  if (!fs.existsSync(filePath)) return false;

  const allowlist = JSON.parse(fs.readFileSync(filePath, "utf8"));
  if (!Object.hasOwn(allowlist, "kandev")) return false;
  if (allowlist.kandev !== COMPACT_KANDEV_EXCEPTION) {
    throw new Error(
      `Refusing to remove unexpected kandev audit exception: ${JSON.stringify(allowlist.kandev)}`,
    );
  }

  delete allowlist.kandev;
  fs.writeFileSync(filePath, `${JSON.stringify(allowlist, null, 2)}\n`);
  return true;
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  const filePath = process.argv[2];
  if (!filePath) {
    console.error("Usage: homebrew-audit-allowlist.mjs <allowlist-path>");
    process.exit(2);
  }
  try {
    const removed = removeCompactKandevAuditException(filePath);
    console.log(
      removed
        ? "Removed the obsolete kandev audit exception."
        : "No obsolete kandev audit exception found.",
    );
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}
