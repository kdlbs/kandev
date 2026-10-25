import { render, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TaskCreateDialogProps } from "./task-create-dialog-types";
import { LinearQuickTaskLauncher } from "./linear/linear-quick-task-launcher";
import { QuickTaskLauncher as JiraQuickTaskLauncher } from "./jira/my-jira/quick-task-launcher";

const mocks = vi.hoisted(() => ({ push: vi.fn(), props: null as TaskCreateDialogProps | null }));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: TaskCreateDialogProps) => {
    mocks.props = props;
    return null;
  },
}));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: mocks.push }) }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it.each(["linear", "jira"])(
  "closes %s task creation without navigating when auto-focus is off",
  (provider) => {
    const onClose = vi.fn();
    const common = {
      workspaceId: "ws",
      workflows: [{ id: "wf" }] as never,
      steps: [{ id: "step", workflow_id: "wf", position: 0 }] as never,
      onClose,
    };
    if (provider === "linear")
      render(
        <LinearQuickTaskLauncher
          {...common}
          issue={{ identifier: "LIN-1", title: "Issue", url: "https://linear.app/issue" } as never}
        />,
      );
    else
      render(
        <JiraQuickTaskLauncher
          {...common}
          payload={
            {
              ticket: { key: "JIRA-1", summary: "Issue" },
              preset: { prompt: () => "Issue" },
            } as never
          }
        />,
      );
    mocks.props?.onSuccess?.({ id: "task" } as never, "create", { autoFocus: false });
    expect(onClose).toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
  },
);
