import { describe, expect, it } from "vitest";

import { resolveActiveOfficeWorkspaceId } from "./office-routes";

describe("resolveActiveOfficeWorkspaceId", () => {
  const wsOffice1 = "ws-office-1";
  const wsOffice2 = "ws-office-2";

  it("prefers explicit route workspace ID", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: "office-flow-1" },
        { id: wsOffice2, office_workflow_id: "office-flow-2" },
      ],
      wsOffice2,
      "ws-office-1",
      null,
      null,
    );

    expect(activeId).toBe(wsOffice2);
  });

  it("falls back to the generic office workspace cookie and then settings", () => {
    const activeId = resolveActiveOfficeWorkspaceId(
      [
        { id: wsOffice1, office_workflow_id: "office-flow-1" },
        { id: wsOffice2, office_workflow_id: "office-flow-2" },
      ],
      null,
      "ws-missing",
      wsOffice1,
      wsOffice2,
    );

    expect(activeId).toBe(wsOffice1);
  });
});
