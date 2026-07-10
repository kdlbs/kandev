import { describe, expect, it } from "vitest";

import { idParamsPromise, resolveActiveOfficeWorkspaceId } from "./office-routes";

describe("resolveActiveOfficeWorkspaceId", () => {
  const wsOffice1 = "ws-office-1";
  const wsOffice2 = "ws-office-2";
  const officeFlow1 = "office-flow-1";
  const officeFlow2 = "office-flow-2";

  it("prefers explicit route workspace ID", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: officeFlow1 },
        { id: wsOffice2, office_workflow_id: officeFlow2 },
      ],
      wsOffice2,
      "ws-office-1",
      null,
      null,
    );

    expect(activeId).toBe(wsOffice2);
  });

  it("falls back to the generic office workspace cookie when active cookie misses", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: officeFlow1 },
        { id: wsOffice2, office_workflow_id: officeFlow2 },
      ],
      null,
      "ws-missing",
      wsOffice1,
      wsOffice2,
    );

    expect(activeId).toBe(wsOffice1);
  });

  it("falls back to settings workspace when no cookie matches", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: officeFlow1 },
        { id: wsOffice2, office_workflow_id: officeFlow2 },
      ],
      null,
      "ws-missing",
      null,
      wsOffice2,
    );

    expect(activeId).toBe(wsOffice2);
  });

  it("uses kandev-active-workspace when it holds an office workspace ID", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: officeFlow1 },
        { id: wsOffice2, office_workflow_id: officeFlow2 },
      ],
      null,
      wsOffice1,
      wsOffice2,
      null,
    );

    expect(activeId).toBe(wsOffice1);
  });
});

describe("idParamsPromise", () => {
  // The helper backs Next-style `params: Promise<{ id }>` props consumed via
  // `use(params)`. Every call site inside `renderOfficeRoute` runs on each
  // render of `OfficeRoutes`, so identity must be stable across calls or the
  // enclosing `<Suspense>` re-suspends forever and hides the office tree.
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
});
