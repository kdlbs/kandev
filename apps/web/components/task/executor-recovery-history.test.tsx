import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ExecutorRecoveryHistory } from "./executor-recovery-history";
afterEach(cleanup);
describe("executor recovery attention", () => {
  it("calls out lost conversation continuity with a distinct warning", () => {
    render(
      <ExecutorRecoveryHistory
        outcome="fresh"
        recoveredAt="2026-10-07T00:00:00Z"
        workspace="retained"
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "A new provider conversation was started",
    );
    expect(screen.getByRole("heading").textContent).toContain("Executor recovery confirmed");
  });
  it("presents restored continuity as status rather than an active error", () => {
    render(<ExecutorRecoveryHistory outcome="restored" recoveredAt="2026-10-07T00:00:00Z" />);
    expect(screen.getByRole("status").textContent).toContain("restored the original conversation");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
