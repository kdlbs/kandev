import { ESLint } from "eslint";
import { beforeAll, describe, expect, it } from "vitest";

const webRoot = process.cwd();
const ruleId = "system-query-owner/no-migrated-system-zustand-owner";

const ownerPaths = {
  types: "lib/state/slices/system/types.ts",
  slice: "lib/state/slices/system/system-slice.ts",
  barrel: "lib/state/slices/system/index.ts",
  hydrator: "lib/state/hydration/hydrator.ts",
};

const queryHookPaths = [
  "hooks/domains/system/use-system-info.ts",
  "hooks/domains/system/use-database-stats.ts",
  "hooks/domains/system/use-backups.ts",
  "hooks/domains/system/use-disk-usage.ts",
];

const positiveFixtures = [
  {
    file: ownerPaths.types,
    code: `type SystemSliceState = { system: { info: string | null } };
type SystemSliceActions = {};`,
    line: 1,
    resource: "About SystemInfo",
    replacement: "useSystemInfo",
  },
  {
    file: ownerPaths.slice,
    code: `const defaultSystemState = { system: { database: null } };`,
    line: 1,
    resource: "database statistics",
    replacement: "useDatabaseStats",
  },
  {
    file: ownerPaths.barrel,
    code: `export type { SystemBackupsState } from "./types";`,
    line: 1,
    resource: "backup list",
    replacement: "useBackups",
  },
  {
    file: ownerPaths.hydrator,
    code: `import { deepMerge } from "./merge-strategies";
export function hydrateState(draft: any) {
  deepMerge(draft.system, { diskUsage: null });
}`,
    line: 3,
    resource: "disk usage",
    replacement: "useDiskUsage",
  },
];

function severity(setting: unknown): number {
  const value = Array.isArray(setting) ? setting[0] : setting;
  if (value === "error" || value === 2) return 2;
  if (value === "warn" || value === 1) return 1;
  return 0;
}

function absolute(file: string): string {
  return `${webRoot}/${file}`;
}

describe("migrated System Query owner ESLint wiring", () => {
  let eslint: ESLint;

  beforeAll(async () => {
    eslint = new ESLint({ cwd: webRoot });
  });

  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.3
  it.each(Object.values(ownerPaths))("runs on owner path %s", async (file) => {
    const config = await eslint.calculateConfigForFile(absolute(file));
    expect(severity(config?.rules[ruleId])).toBe(2);
  });

  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.3
  it.each(positiveFixtures)(
    "finds a supported mirror in $file",
    async ({ file, code, line, resource, replacement }) => {
      const [result] = await eslint.lintText(code, { filePath: absolute(file) });
      const finding = result.messages.find((message) => message.ruleId === ruleId);

      expect(finding).toBeDefined();
      expect(finding).toMatchObject({ ruleId, severity: 2, line });
      expect(finding?.message).toContain(resource);
      expect(finding?.message).toContain(replacement);
    },
  );

  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.3
  it("keeps the current production owner files clean", async () => {
    const results = await eslint.lintFiles(Object.values(ownerPaths).map(absolute));
    const findings = results.flatMap((result) =>
      result.messages.filter((message) => message.ruleId === ruleId),
    );

    expect(findings).toEqual([]);
  });

  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.4
  it.each(queryHookPaths)("does not register on Query hook %s", async (file) => {
    const config = await eslint.calculateConfigForFile(absolute(file));
    expect(severity(config?.rules[ruleId])).toBe(0);
  });

  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.2
  it.each([
    {
      file: ownerPaths.types,
      code: `import type { SystemInfo, DatabaseStats, SnapshotInfo, DiskUsageResponse } from "@/lib/types";
type SystemSliceState = { system: { jobs: {}; metrics: null; updates: null; retention: null; storage: { disk: null } } };
type SystemSliceActions = { upsertSystemJob: (job: unknown) => void };`,
    },
    {
      file: ownerPaths.slice,
      code: `const defaultSystemState = { system: { jobs: {}, metrics: null, updates: null, retention: null, storage: { disk: null, policy: null, overview: null, runs: [], quarantine: [] } } };
const unrelated = { system: { info: null, database: null, backups: [], diskUsage: null } };
const createSystemSlice = (set: any) => ({ update: () => set((draft: any) => {
  const system = { info: null };
  system.info = null;
  const localDraft = { system: { database: null } };
  localDraft.system.database = null;
  function nested(draft: any, system: any) {
    draft.system.backups = [];
    system.diskUsage = null;
  }
}) });
// Retired-key text in comments and strings is not a state owner.
const note = "system.info database backups diskUsage";
const template = \`system.info \${"database"} backups diskUsage\`;
const uiDraft = { settingsDraft: { value: "local" }, mutationFeedback: null };
const queryRead = () => useSystemInfo();
const processProbe = () => fetch("/api/v1/system/processes?no-store=true");`,
    },
    {
      file: ownerPaths.barrel,
      code: `export type { SystemSliceState, SystemSliceActions } from "./types";
const unrelated = { SystemBackupsState: true };`,
    },
    {
      file: ownerPaths.hydrator,
      code: `import { deepMerge } from "./merge-strategies";
export function hydrateState(draft: any, state: any) {
  deepMerge(draft.system, state.system);
  const unrelated = { system: { info: null, nested: { database: null }, storage: { disk: null } } };
  function nested(draft: any) {
    draft.system.info = null;
    deepMerge(draft.system, { database: null });
  }
  function nestedShadow(draft: any, system: any) {
    draft.system.backups = [];
    system.diskUsage = null;
  }
  const later = () => { draft.system.backups = []; };
  const system = { diskUsage: null };
  system.diskUsage = null;
  {
    const draft = { system: { diskUsage: null } };
    deepMerge(draft.system, { info: null });
  }
  {
    const deepMerge = (_target: unknown, _payload: unknown) => {};
    deepMerge(draft.system, { info: null });
  }
}`,
    },
  ])("allows unrelated or shadowed shape in $file", async ({ file, code }) => {
    const [result] = await eslint.lintText(code, { filePath: absolute(file) });
    expect(result.messages.filter((message) => message.ruleId === ruleId)).toEqual([]);
  });
});
