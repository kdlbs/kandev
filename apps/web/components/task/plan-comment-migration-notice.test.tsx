import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { planCommentRecovery } from "@/lib/plan-comment-recovery";
import { PlanCommentMigrationNotice } from "./plan-comment-migration-notice";

afterEach(cleanup);

describe("plan comment recovery feedback", () => {
  it.each(["idle", "running", "retrying", "complete", "failed"] as const)(
    "stays quiet for empty %s recovery",
    (status) => {
      render(
        <PlanCommentMigrationNotice
          {...planCommentRecovery({ status, pendingCount: 0, failure: null })}
          retry={vi.fn()}
        />,
      );
      expect(screen.queryByTestId("plan-comment-migration-notice")).toBeNull();
    },
  );

  it("stays quiet while real drafts retry automatically", () => {
    render(
      <PlanCommentMigrationNotice
        {...planCommentRecovery({ status: "retrying", pendingCount: 1, failure: "transient" })}
        retry={vi.fn()}
      />,
    );
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByTestId("plan-comment-migration-notice")).toBeNull();
  });

  it("offers explicit Retry for identified feedback needing attention", () => {
    const retry = vi.fn();
    render(
      <PlanCommentMigrationNotice
        {...planCommentRecovery({ status: "failed", pendingCount: 1, failure: "conflict" })}
        retry={retry}
      />,
    );
    expect(screen.getByRole("alert")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
