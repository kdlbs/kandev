/**
 * Query options barrel — re-exports all domain-specific queryOptions() factories.
 *
 * Waves 1–5 workers populate this file by adding per-domain exports, e.g.:
 *
 *   export * from "./features";
 *   export * from "./workspaces";
 *   export * from "./kanban";
 *   export * from "./session";
 *   export * from "./office";
 *   export * from "./github";
 *   export * from "./gitlab";
 *   export * from "./jira";
 *   export * from "./linear";
 *
 * Each domain file co-locates its queryOptions() with the corresponding
 * key factories in lib/query/keys.ts so SSR prefetch and CSR useQuery
 * share the same options object.
 */
