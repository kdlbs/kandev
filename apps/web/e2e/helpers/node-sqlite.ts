// Playwright's ESM loader in Node 24 cannot load the experimental node:sqlite
// source directly. Runtime builtin lookup works across the supported Node 24
// versions and keeps the E2E fixtures independent of the sqlite CLI.
const sqlite = process.getBuiltinModule("node:sqlite") as typeof import("node:sqlite") | undefined;
if (!sqlite) throw new Error("this E2E suite requires the Node.js SQLite builtin");

export const { DatabaseSync } = sqlite;
