import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ControlUnattributedUsage } from "./control-unattributed-usage";
afterEach(cleanup);
describe("unattributed execution evidence", () => {
  it("compacts repeated reports without inventing an account, utilization or selected provider", () => {
    const entries = [1, 2, 3].map((id) => ({
      execution_id: `execution-${id}`,
      session_id: `session-${id}`,
      report: {
        model: "opus",
        source_known: false,
        limited: false,
        reason: "provider_selection_unknown",
      },
    }));
    render(<ControlUnattributedUsage entries={entries} />);
    expect(screen.getAllByTestId("control-unattributed-model")).toHaveLength(1);
    expect(screen.getByRole("heading").textContent).toContain("3");
    expect(screen.getByTestId("control-unattributed-model").textContent).toContain("opus");
    expect(screen.getByTestId("control-unattributed-model").textContent).toContain(
      "provider_selection_unknown",
    );
    expect(screen.queryByTestId("control-provider-account")).toBeNull();
    expect(screen.getByTestId("control-unattributed-usage").textContent).not.toContain("0%");
    expect(screen.getByTestId("control-unattributed-usage").querySelectorAll("li")).toHaveLength(3);
  });
});
