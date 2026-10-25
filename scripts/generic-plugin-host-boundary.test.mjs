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

function splitMarkdownTableRow(line) {
  const cells = [];
  let cell = "";
  for (let index = 1; index < line.length - 1; index += 1) {
    if (line[index] === "\\" && line[index + 1] === "|") {
      cell += "|";
      index += 1;
    } else if (line[index] === "|") {
      cells.push(cell.trim());
      cell = "";
    } else {
      cell += line[index];
    }
  }
  cells.push(cell.trim());
  return cells;
}

function tableRows(document, start, end) {
  return section(document, start, end)
    .split("\n")
    .filter((line) => line.startsWith("|"))
    .map(splitMarkdownTableRow);
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
  assert.match(adr, /### H6 capability context bootstrap/);
  assert.match(adr, /`GetCapabilityContext`/);
  assert.match(adr, /capability-context-changed/);
  assert.match(adr, /### H1 result mapping/);
  assert.doesNotMatch(adr, /SetTaskFlagsExact|flag requests/);

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
  const inventoryRows = inventory
    .split("\n")
    .filter((line) => line.startsWith("| H"));
  const methodAndCapabilityColumns = inventoryRows
    .map((line) => {
      const cells = line.split("|");
      return `${cells[3]}|${cells[7]}`;
    })
    .join("\n");

  assert.doesNotMatch(
    methodAndCapabilityColumns,
    /\bmerge(?:[A-Z][A-Za-z0-9_]*)?\b/i,
  );
  assert.doesNotMatch(methodAndCapabilityColumns, /api_write:merge/i);
  assert.doesNotMatch(inventory, /api_(?:read|write):[^`\s|]*coordinator/i);
  assert.match(inventory, /No Host writer is approved in H0/);
});

test("preserves literal pipes inside decision-table cells", async () => {
  const adr = await readADR();
  const inventoryRows = tableRows(
    adr,
    "### Public Host surface inventory",
    "### H2d exact-head evidence",
  );
  const capabilityRows = tableRows(
    adr,
    "The smallest extension is owned by the plugin system:",
    "No Coordinator principal, role, profile, grant, or audit type is created.",
  );

  assert.ok(inventoryRows.length > 1, "missing Host inventory table");
  assert.ok(capabilityRows.length > 1, "missing capability contract table");
  assert.ok(
    inventoryRows.every((row) => row.length === 9),
    "Host inventory row was split by a literal pipe",
  );
  assert.ok(
    capabilityRows.every((row) => row.length === 2),
    "capability contract row was split by a literal pipe",
  );
  assert.match(adr, /task_directive\.issued\\\|resolved\\\|revoked/);
  assert.match(adr, /task_relation\.added\\\|removed/);
  assert.match(adr, /status `active\\\|revoked`/);
});

test("freezes the complete deferred H3d decision table", async () => {
  const adr = await readADR();
  const rows = tableRows(
    adr,
    "### H3d provider-action decision",
    "### Versioning and compatibility",
  );
  const headers = rows[0].join(" ");

  assert.equal(rows[0].length, 7, "H3d table must expose all decision fields");
  assert.ok(
    rows.slice(1).every((row) => row.length === 7),
    "H3d row has malformed columns",
  );
  for (const field of [
    "exact-head/current-state",
    "Idempotency",
    "Capability",
    "Audit receipt",
    "readback",
  ]) {
    assert.match(headers, new RegExp(field, "i"), `H3d table missing ${field}`);
  }
  assert.doesNotMatch(
    adr.slice(
      adr.indexOf("### H3d provider-action decision"),
      adr.indexOf("### Versioning and compatibility"),
    ),
    /\| Merge \|/i,
  );
});

test("fences legacy v1 calls from H6 exact authority", async () => {
  const adr = await readADR();
  const legacy = section(
    adr,
    "### Legacy v1 compatibility fence",
    "### Public Host surface inventory",
  );

  for (const required of [
    "GetConfig(GetConfigRequest {})",
    "GetConfigRequest {})` | Ungated, plugin-global operator configuration",
    "ListTasks",
    "ListSessions",
    "CreateTask",
    "api_read:<resource>",
    "api_write:<resource>",
    "No synthetic legacy revision exists.",
    "cannot authorize any new exact capability or bypass H6/C1/C2 safeguards",
  ]) {
    assert.ok(legacy.includes(required), `missing legacy fence: ${required}`);
  }
  assert.match(
    adr,
    /Every \*\*new exact\*\* Host request introduced by this ADR carries/,
  );
  assert.match(adr, /host\.v2\.read:tasks/);
  assert.match(adr, /host\.v2\.write:tasks/);
  assert.doesNotMatch(legacy, /synthetic legacy revision only/i);
});

test("defines approval lifetime, result parents, and principal-correct parity", async () => {
  const adr = await readADR();

  for (const lifecycle of ["Upgrade", "Rollback", "Uninstall", "Reinstall"]) {
    assert.match(
      adr,
      new RegExp(`\\| ${lifecycle} \\|`),
      `missing installation lifecycle ${lifecycle}`,
    );
  }
  assert.match(adr, /Mints a fresh `installation_id`/);
  assert.match(adr, /tombstones the `installation_id`/);
  assert.match(
    adr,
    /\| `DENIED` \| `STALE_CAPABILITY_REVISION`, `CAPABILITY_REVOKED`, `HUMAN_RESERVED`/,
  );
  assert.match(
    adr,
    /\| `CONFLICT` \| `STALE_RESOURCE_VERSION`, `PENDING_TRANSITION_CONFLICT`, `EXECUTION_GENERATION_FENCED`/,
  );
  assert.match(
    adr,
    /Host RPC and global MCP authorization is tested separately before parity/,
  );
  assert.match(
    adr.replace(/\s+/g, " "),
    /A Host approval denial is never expected to equal an MCP authorization verdict/,
  );
});

test("assigns pending-transition reads only to H2c", async () => {
  const adr = await readADR();
  const inventory = section(
    adr,
    "### Public Host surface inventory",
    "### H2d exact-head evidence",
  );
  const h5Row = inventory.split("\n").find((line) => line.startsWith("| H5 "));

  assert.ok(h5Row, "missing H5 inventory row");
  assert.match(h5Row, /CancelPendingTaskTransitionExact/);
  assert.doesNotMatch(
    h5Row,
    /ListPendingTaskTransitions|TaskTransitionQuery|api_read:/,
  );
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
