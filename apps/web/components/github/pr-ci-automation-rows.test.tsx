import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { CIAutomationQueueStatusRow } from "./pr-ci-automation-rows";

afterEach(() => cleanup());

describe("CI automation queue recovery status", () => {
  it("renders active queue status as non-interactive status text", () => {
    const { getByRole } = render(
      <CIAutomationQueueStatusRow
        queueState={{
          context: "queued",
          status: "queued",
          removalCause: "unknown",
          repairAccepted: false,
          waitingForCommit: false,
        }}
      />,
    );

    expect(getByRole("status").textContent).toContain("Active merge queue attempt");
  });

  it("renders the same-head guard status after removal", () => {
    const { getByRole } = render(
      <CIAutomationQueueStatusRow
        queueState={{
          context: "recovery",
          status: "waiting_for_commit",
          removalCause: "checks_failed",
          repairAccepted: false,
          waitingForCommit: true,
        }}
      />,
    );

    expect(getByRole("status").textContent).toContain("Waiting for a new commit");
  });
});
