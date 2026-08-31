import assert from "node:assert/strict";
import fs from "node:fs/promises";
import test from "node:test";

const adrPath = "docs/decisions/2026-08-31-generic-plugin-host-boundary.md";

async function readADR() {
  return fs.readFile(adrPath, "utf8");
}

function section(document, start, end) {
  const startIndex = document.indexOf(start);
  const endIndex = document.indexOf(end, startIndex + start.length);
  assert.notEqual(startIndex, -1, `missing section ${start}`);
  assert.notEqual(endIndex, -1, `missing section ${end}`);
  return document.slice(startIndex, endIndex);
}

function occurrenceCount(document, value) {
  return document.split(value).length - 1;
}

test("freezes every H0 Host unit and correction", async () => {
  const adr = await readADR();

  for (const unit of [
    "H1",
    "H2a",
    "H2b",
    "H2c",
    "H2d",
    "H2e",
    "H3a",
    "H3b",
    "H3c",
    "H3d",
    "H4",
    "H5",
    "H7",
  ]) {
    assert.match(
      adr,
      new RegExp(`\\| ${unit}(?: | \\|)`),
      `missing ${unit} inventory row`,
    );
  }
  for (let correction = 1; correction <= 7; correction += 1) {
    assert.match(
      adr,
      new RegExp(`^### C${correction}:`, "m"),
      `missing C${correction}`,
    );
  }

  assert.match(adr, /Decision: \*\*`H6_REQUIRED`\*\*\./);
  assert.match(adr, /H3d is explicitly deferred\./);

  for (const contract of [
    "`PluginCapabilityApproval`",
    "`PluginCapabilityApprovalEvent`",
    "manifest declaration ∩ current workspace approval ∩ immutable Human policy",
  ]) {
    assert.match(adr, new RegExp(contract), `missing H6 contract ${contract}`);
  }
  for (const candidate of [
    "Retry or rerun",
    "Draft to ready",
    "Reviewer notification or re-request",
  ]) {
    assert.match(
      adr,
      new RegExp(`\\| ${candidate} \\| Deferred \\|`),
      `missing H3d candidate ${candidate}`,
    );
  }
});

test("keeps the public Host inventory generic and merge-free", async () => {
  const adr = await readADR();
  const inventory = section(
    adr,
    "### Public Host surface inventory",
    "### H2d exact-head evidence",
  );

  assert.doesNotMatch(inventory, /coordinator/i);
  assert.doesNotMatch(inventory, /\bMerge[A-Z][A-Za-z]*\b/);
  assert.doesNotMatch(inventory, /`(?:[A-Za-z0-9_.]*\.)?merge[A-Za-z0-9_]*`/i);
  assert.doesNotMatch(inventory, /`api_write:[^`]*merge[^`]*`/i);
  assert.doesNotMatch(inventory, /api_(?:read|write):[^`\s|]*coordinator/i);
  assert.match(inventory, /No Host writer is approved in H0/);
});

test("pins immutable migration evidence and forbidden integration paths", async () => {
  const adr = await readADR();
  const normalized = adr.replace(/\s+/g, " ");

  assert.equal(
    occurrenceCount(adr, "afd2b699bfe9b6af9353ea01728582f61a7be2be"),
    2,
    "host baseline head must remain consistent in context and migration ledger",
  );
  assert.equal(
    occurrenceCount(adr, "5bfdbcf7d9608d1210453f95ebfc8f66c3179225"),
    2,
    "plugin baseline head must remain consistent in context and migration ledger",
  );
  for (const forbidden of [
    "ambient/global Kandev MCP",
    "private or undocumented REST",
    "direct database access",
    "shelling into Kandev",
  ]) {
    assert.match(normalized, new RegExp(forbidden.replace("/", "\\/")));
  }
});
