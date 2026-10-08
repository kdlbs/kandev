import { RuleTester } from "eslint";
import tseslint from "typescript-eslint";
import { describe, it } from "vitest";

import { noMigratedSystemQueryOwner } from "./no-migrated-system-query-owner.mjs";

RuleTester.describe = describe;
RuleTester.it = it;

const ruleTester = new RuleTester({
  languageOptions: {
    parser: tseslint.parser as never,
    parserOptions: { ecmaVersion: 2022, sourceType: "module" },
  },
});

const backupListResource = "backup list";
const systemInfoResource = "About SystemInfo";
const systemInfoHook = "useSystemInfo";

const diagnostic = (resource: string, replacement: string) => ({
  messageId: "migratedSystemQueryOwner",
  data: { resource, replacement },
});

const owners = [
  { field: "info", action: "setSystemInfo", resource: systemInfoResource, hook: systemInfoHook },
  {
    field: "database",
    action: "setSystemDatabase",
    resource: "database statistics",
    hook: "useDatabaseStats",
  },
  {
    field: "backups",
    action: "setSystemBackups",
    resource: backupListResource,
    hook: "useBackups",
  },
  {
    field: "diskUsage",
    action: "setSystemDiskUsage",
    resource: "disk usage",
    hook: "useDiskUsage",
  },
];

ruleTester.run("no-migrated-system-zustand-owner", noMigratedSystemQueryOwner, {
  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.2
  valid: [
    {
      code: `import type { SystemInfo, DatabaseStats, SnapshotInfo, DiskUsageResponse, SystemBackupsState } from "@/lib/types";
        type SystemSliceState = { system: { retention: null; updates: null; jobs: {}; metrics: null; storage: { policy: null; overview: null; analysisRevision: number; disk: null; diskIdentity: null; runs: []; quarantine: [] } } };
        type SystemSliceActions = { setSystemRetention: (value: unknown) => void; upsertSystemJob: (value: unknown) => void };
        const unrelated = { system: { info: null, database: null, backups: [], diskUsage: null } };`,
    },
    {
      code: `type OtherState = { system: { info: null; database: null; backups: []; diskUsage: null } };
        const defaultOtherState = { system: { info: null, database: null, backups: [], diskUsage: null } };`,
    },
    {
      code: `const defaultSystemState = { system: { jobs: {}, metrics: null, updates: null, retention: null, storage: { disk: null, policy: null, overview: null, analysisRevision: 0, diskIdentity: null, runs: [], quarantine: [] } } };`,
    },
    {
      code: `const createSystemSlice = (set: any) => ({
          setSystemJob: () => { const unrelated = { system: { info: null } }; unrelated.system.info = "local"; },
          shadowedSet: (set: any) => set((draft: any) => { draft.system.info = null; }),
          nestedCallback: () => set((draft: any) => { queueMicrotask(() => { draft.system.database = null; }); }),
          update: () => set((draft: any) => {
            const system = { diskUsage: null };
            system.diskUsage = null;
            const localDraft = { system: { backups: [] } };
            localDraft.system.backups = [];
            function nested(draft: any, system: any) {
              draft.system.info = null;
              system.database = null;
            }
          }),
        });`,
    },
    {
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any, state: any) {
          deepMerge(draft.system, state.system);
          deepMerge(draft.system, { nested: { info: null } });
          deepMerge({ system: { info: null } }, { database: null });
          const other = { system: { backups: [] } };
          const shadowedDraft = { system: { diskUsage: null } };
          deepMerge(shadowedDraft.system, { info: null });
          function nested(draft: any) { draft.system.info = null; deepMerge(draft.system, { database: null }); }
          const callback = () => { draft.system.backups = []; };
          function nested(draft: any, system: any) {
            draft.system.info = null;
            system.database = null;
          }
          const system = { diskUsage: null };
          system.diskUsage = null;
          {
            const deepMerge = (_target: unknown, _payload: unknown) => {};
            deepMerge(draft.system, { diskUsage: null });
          }
        }`,
    },
    {
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any) {
          draft.system[field] = null;
          draft.system[\`\${field}\`] = null;
          deepMerge(draft.system, { [field]: null, [\`\${field}\`]: null });
          deepMerge(draft.system, { ...candidate });
        }`,
    },
    {
      code: `export type { SystemSliceState, SystemSliceActions } from "./types";
        const message = "SystemBackupsState info database backups diskUsage";
        const note = \`SystemBackupsState \${"info"}\`;`,
    },
  ],
  // @covers AC-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001.1
  invalid: [
    ...owners.map(({ field, resource, hook }) => ({
      code: `type SystemSliceState = { system: { ${field}: unknown } };`,
      errors: [diagnostic(resource, hook)],
    })),
    ...owners.map(({ action, resource, hook }) => ({
      code: `type SystemSliceActions = { ${action}: () => void };`,
      errors: [diagnostic(resource, hook)],
    })),
    {
      code: `type SystemSliceActions = { ["setSystemInfo"]: () => void };`,
      errors: [diagnostic(systemInfoResource, systemInfoHook)],
    },
    {
      code: `type SystemBackupsState = { items: SnapshotInfo[]; loaded: boolean };`,
      errors: [diagnostic(backupListResource, "useBackups")],
    },
    ...owners.map(({ field, resource, hook }) => ({
      code: `const defaultSystemState = { system: { ${field}: null } };`,
      errors: [diagnostic(resource, hook)],
    })),
    {
      code: `const defaultSystemState = { system: { "backups": [] } };`,
      errors: [diagnostic(backupListResource, "useBackups")],
    },
    {
      code: `const defaultSystemState = { system: { ["diskUsage"]: null } };`,
      errors: [diagnostic("disk usage", "useDiskUsage")],
    },
    ...owners.map(({ action, resource, hook }) => ({
      code: `const createSystemSlice = (set: any) => ({ ${action}: () => undefined });`,
      errors: [diagnostic(resource, hook)],
    })),
    {
      code: `const createSystemSlice = (set: any) => ({ ["setSystemDatabase"]: () => undefined });`,
      errors: [diagnostic("database statistics", "useDatabaseStats")],
    },
    ...owners.map(({ field, resource, hook }) => ({
      code: `const createSystemSlice = (set: any) => ({ update: () => set((draft: any) => { draft.system.${field} = null; }) });`,
      errors: [diagnostic(resource, hook)],
    })),
    {
      code: `const createSystemSlice = (set: any) => ({ update: () => set((draft: any) => { draft["system"]["info"] = null; }) });`,
      errors: [diagnostic(systemInfoResource, systemInfoHook)],
    },
    ...owners.map(({ field, resource, hook }) => ({
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any) { draft.system.${field} = null; }`,
      errors: [diagnostic(resource, hook)],
    })),
    ...owners.map(({ field, resource, hook }) => ({
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any) { deepMerge(draft.system, { ${field}: null }); }`,
      errors: [diagnostic(resource, hook)],
    })),
    {
      code: `type SystemSliceState = { system: { ["info"]: unknown } };`,
      errors: [diagnostic(systemInfoResource, systemInfoHook)],
    },
    {
      code: `type SystemSliceState = { system: { "database": unknown } };`,
      errors: [diagnostic("database statistics", "useDatabaseStats")],
    },
    {
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any) { draft.system["backups"] = []; }`,
      errors: [diagnostic(backupListResource, "useBackups")],
    },
    {
      code: `import { deepMerge } from "./merge-strategies";
        export function hydrateState(draft: any) { deepMerge(draft.system, { ["diskUsage"]: null }); }`,
      errors: [diagnostic("disk usage", "useDiskUsage")],
    },
    {
      code: `export type { SystemBackupsState } from "./types";`,
      errors: [diagnostic(backupListResource, "useBackups")],
    },
    {
      code: `export { type SystemBackupsState as LegacyBackups } from "./types";`,
      errors: [diagnostic(backupListResource, "useBackups")],
    },
  ],
});
