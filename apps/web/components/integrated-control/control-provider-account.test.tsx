import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ControlProviderAccount } from "./control-provider-account";
import { groupExecutionUsage } from "@/lib/control-ui/usage";
import type { ProfileExecutionUsage } from "@/lib/api/domains/provider-usage-api";

afterEach(cleanup);

describe("provider/account presentation", () => {
  it("shows measured zero separately from unobserved values and uses the resolved model", () => {
    const entry: ProfileExecutionUsage = {
      execution_id: "e",
      session_id: "s",
      report: {
        source_known: true,
        reason: "",
        model: "sol",
        limited: false,
        current: {
          provider: "openai",
          account_ref: "account-one",
          fetched_at: "2026-10-18T14:00:00Z",
          windows: [
            {
              label: "5 hours",
              utilization_pct: 0,
              utilization_observed: true,
              reset_at: "",
              reset_at_observed: false,
            },
            {
              label: "7 days",
              utilization_pct: 0,
              utilization_observed: false,
              reset_at: "",
              reset_at_observed: false,
            },
          ],
        },
      },
    };
    render(<ControlProviderAccount group={groupExecutionUsage([entry])[0]} />);
    const windows = screen.getAllByTestId("control-provider-window");
    expect(windows[0].textContent).toContain("0%");
    expect(windows[1].textContent).not.toContain("0%");
    expect(screen.getByTestId("control-provider-account").textContent).toContain("account-one");
    expect(screen.getByTestId("control-provider-model").textContent).toContain("sol");
    expect(screen.queryByText("Dynamic")).toBeNull();
  });
});
