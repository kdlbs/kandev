import { beforeEach, describe, expect, it, vi } from "vitest";

// The scoped cookie helpers derive the default port from the API base URL;
// pin it so scoped-name fixtures are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

import {
  __resetIdParamsPromiseCacheForTests,
  idParamsPromise,
  resolveOfficeBootstrapWorkspaceId,
} from "./office-routes";

describe("resolveOfficeBootstrapWorkspaceId (OfficeRoutes cookie reads)", () => {
  const wsOffice1 = "ws-office-1";
  const wsOffice2 = "ws-office-2";
  const wsKanban = "ws-kanban-1";
  const items = [
    { id: wsOffice1, office_workflow_id: "office-flow-1" },
    { id: wsOffice2, office_workflow_id: "office-flow-2" },
  ];

  beforeEach(() => {
    document.cookie = "kandev-active-workspace=; path=/; max-age=0";
    document.cookie = "office-active-workspace=; path=/; max-age=0";
    document.cookie = "kandev-active-workspace_8443=; path=/; max-age=0";
    document.cookie = "office-active-workspace_8443=; path=/; max-age=0";
  });

  it("route override beats cookies naming a different workspace (invariant)", () => {
    document.cookie = "kandev-active-workspace_8443=ws-office-1; path=/";
    document.cookie = "office-active-workspace_8443=ws-office-1; path=/";

    expect(resolveOfficeBootstrapWorkspaceId(items, wsOffice2, null)).toBe(wsOffice2);
  });

  it("hydrates the scoped general cookie with no route or unprefixed cookie (failing-before)", () => {
    // Non-first fixture: pre-change the reader only sees unprefixed names, so
    // this resolves to the first office workspace instead of ws-office-2.
    document.cookie = "kandev-active-workspace_8443=ws-office-2; path=/";

    expect(resolveOfficeBootstrapWorkspaceId(items, null, null)).toBe(wsOffice2);
  });

  it("prefers the scoped office cookie when the general cookie names a kanban workspace (failing-before)", () => {
    // Pre-change the effect reads only the unprefixed office cookie (absent),
    // so this falls to settings/first instead of ws-office-2.
    document.cookie = `kandev-active-workspace_8443=${wsKanban}; path=/`;
    document.cookie = "office-active-workspace_8443=ws-office-2; path=/";

    expect(resolveOfficeBootstrapWorkspaceId(items, null, null)).toBe(wsOffice2);
  });

  it("falls back to the legacy unprefixed office cookie when no scoped one exists", () => {
    document.cookie = "office-active-workspace=ws-office-2; path=/";

    expect(resolveOfficeBootstrapWorkspaceId(items, null, null)).toBe(wsOffice2);
  });

  it("resolves settings, then first, when no cookie names an office workspace", () => {
    document.cookie = `kandev-active-workspace_8443=${wsKanban}; path=/`;

    expect(resolveOfficeBootstrapWorkspaceId(items, null, wsOffice2)).toBe(wsOffice2);
    expect(resolveOfficeBootstrapWorkspaceId(items, null, null)).toBe(wsOffice1);
  });
});

describe("idParamsPromise", () => {
  // The helper backs Next-style `params: Promise<{ id }>` props consumed via
  // `use(params)`. Every call site inside `renderOfficeRoute` runs on each
  // render of `OfficeRoutes`, so identity must be stable across calls or the
  // enclosing `<Suspense>` re-suspends forever and hides the office tree.
  beforeEach(() => {
    __resetIdParamsPromiseCacheForTests();
  });

  it("returns the same promise instance for the same id", () => {
    const a = idParamsPromise("agent-123");
    const b = idParamsPromise("agent-123");
    expect(a).toBe(b);
  });

  it("returns distinct promises for different ids", () => {
    const a = idParamsPromise("agent-123");
    const b = idParamsPromise("agent-456");
    expect(a).not.toBe(b);
  });

  it("resolves to an object with the requested id", async () => {
    await expect(idParamsPromise("agent-789")).resolves.toEqual({ id: "agent-789" });
  });

  it("re-inserting a cached id after eviction returns a fresh, still-stable promise", () => {
    // FIFO eviction runs at MAX_ID_PARAMS_PROMISE_CACHE = 500 entries. Fill
    // past that with unique ids, then re-request one of the earliest ids;
    // it should have been evicted (new identity) but the *new* identity must
    // still be stable on subsequent calls.
    const first = idParamsPromise("evict-target");
    for (let i = 0; i < 600; i++) {
      idParamsPromise(`fill-${i}`);
    }
    const refetched = idParamsPromise("evict-target");
    expect(refetched).not.toBe(first);
    expect(idParamsPromise("evict-target")).toBe(refetched);
  });
});
