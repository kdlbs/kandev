import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";

import { removeCompactKandevAuditException } from "./homebrew-audit-allowlist.mjs";

function withAllowlist(contents, run) {
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "homebrew-allowlist-test-"),
  );
  const filePath = path.join(directory, "mismatched_binary_allowlist.json");
  try {
    fs.writeFileSync(filePath, `${JSON.stringify(contents, null, 2)}\n`);
    return run(filePath);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
}

test("removes only the known compact Kandev foreign-binary exception", () => {
  withAllowlist(
    {
      kandev: "libexec/bin/agentctl-{darwin,linux}-{amd64,arm64}",
      other_formula: "libexec/bin/foreign-helper",
    },
    (filePath) => {
      assert.equal(removeCompactKandevAuditException(filePath), true);
      assert.deepEqual(JSON.parse(fs.readFileSync(filePath, "utf8")), {
        other_formula: "libexec/bin/foreign-helper",
      });
      assert.equal(removeCompactKandevAuditException(filePath), false);
    },
  );
});

test("refuses to remove an unexpected Kandev audit exception", () => {
  withAllowlist({ kandev: "libexec/bin/agentctl-custom" }, (filePath) => {
    const before = fs.readFileSync(filePath, "utf8");
    assert.throws(
      () => removeCompactKandevAuditException(filePath),
      /Refusing to remove unexpected kandev audit exception/,
    );
    assert.equal(fs.readFileSync(filePath, "utf8"), before);
  });
});

test("does nothing when the tap has already removed the Kandev exception", () => {
  withAllowlist({ other_formula: "libexec/bin/foreign-helper" }, (filePath) => {
    assert.equal(removeCompactKandevAuditException(filePath), false);
  });
});
