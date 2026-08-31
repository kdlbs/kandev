import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";

afterEach(cleanup);

describe("TaskResetEnvConfirmDialog", () => {
  it("renders structured warning copy through one left-aligned description", () => {
    render(
      <TaskResetEnvConfirmDialog open onOpenChange={vi.fn()} hasWorktreePath onConfirm={vi.fn()} />,
    );

    const dialog = screen.getByRole("alertdialog", { name: "Reset environment?" });
    const description = dialog.querySelector('[data-slot="alert-dialog-description"]');
    expect(description).not.toBeNull();
    expect(description?.id).toBe(dialog.getAttribute("aria-describedby"));
    expect(description?.classList.contains("min-w-0")).toBe(true);
    expect(description?.classList.contains("text-left")).toBe(true);
    expect(description?.querySelectorAll("p")).toHaveLength(2);
    expect(description?.textContent).toContain("Any uncommitted or unpushed changes");
  });
});
