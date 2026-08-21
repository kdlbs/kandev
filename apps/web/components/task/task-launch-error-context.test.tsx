import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { TaskLaunchErrorProvider, useTaskLaunchErrorContext } from "./task-launch-error-context";

const { toastMock } = vi.hoisted(() => ({
  toastMock: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

const initialSummary: TaskStatusSummary = {
  revision: 1,
  updated_at: "2026-08-20T00:00:00Z",
  active_error: {
    stamp: "initial",
    occurred_at: "2026-08-20T00:00:00Z",
    preview: "initial error",
    category: "base_branch_missing",
  },
};

function SummaryStamp() {
  const context = useTaskLaunchErrorContext();
  return (
    <span data-testid="summary-stamp">{context?.statusSummary?.active_error?.stamp ?? "none"}</span>
  );
}

function renderProvider(statusSummary: TaskStatusSummary) {
  return render(
    <TaskLaunchErrorProvider
      value={{ taskId: "task-1", workspaceId: "workspace-1", statusSummary }}
    >
      <SummaryStamp />
    </TaskLaunchErrorProvider>,
  );
}

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  toastMock.mockReset();
});

describe("TaskLaunchErrorProvider", () => {
  it.each(["provider_auth_required", "model_capacity"])(
    "does not toast for ordinary active error category %s",
    (category) => {
      renderProvider({
        ...initialSummary,
        active_error: { ...initialSummary.active_error!, category, stamp: `ordinary-${category}` },
      });

      expect(toastMock).not.toHaveBeenCalled();
    },
  );

  it("keeps the hydrated status summary as the initial context value", () => {
    renderProvider(initialSummary);

    expect(screen.getByTestId("summary-stamp").textContent).toBe("initial");
  });
});
