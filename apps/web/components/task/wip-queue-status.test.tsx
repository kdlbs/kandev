import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type Task = KanbanState["tasks"][number];
const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";
const STEP_ID = "review";
const queuedTask: Task = {
  id: "queued-task",
  workspaceId: WORKSPACE_ID,
  workflowId: WORKFLOW_ID,
  workflowStepId: STEP_ID,
  queuedForStepId: STEP_ID,
  wipAdmitted: false,
  title: "Queued task",
  position: 1,
  priority: "medium",
  queuedAt: "2026-09-18T10:00:00Z",
  createdAt: "2026-09-18T09:00:00Z",
};

const state = {
  kanbanMulti: {
    snapshots: {
      [WORKFLOW_ID]: {
        workflowId: WORKFLOW_ID,
        workflowName: "Development",
        steps: [{ id: STEP_ID, title: "Review", color: "blue", position: 1, wip_limit: 2 }],
        tasks: [queuedTask],
      },
    },
  },
  kanban: { workflowId: WORKFLOW_ID, tasks: [], steps: [] },
  workflows: { items: [{ id: WORKFLOW_ID, workspaceId: WORKSPACE_ID, name: "Development" }] },
};

const translationLabels: Record<string, string | ((values?: Record<string, unknown>) => string)> = {
  "task:wipQueueTitle": "Workflow WIP limit",
  "task:launchQueueLabel": "Queued",
  "task:wipQueueDestination": (values) => `${values?.workflow ?? ""} / ${values?.step ?? ""}`,
  "task:wipQueuePosition": (values) =>
    `${values?.position ?? ""} of ${values?.total ?? ""} tasks waiting for this step.`,
  "task:wipQueueAdmittedCount": (values) =>
    `${values?.count ?? ""} of ${values?.limit ?? ""} tasks admitted.`,
  "task:wipQueueAdmittedUnlimited": (values) => `${values?.count ?? ""} tasks admitted.`,
  "task:wipQueueConfigure": "Configure workflow WIP limit",
  "task:wipQueueUnavailable": "Workflow WIP details are unavailable.",
  "task:wipQueueConfigurationUnavailable": "The workflow configuration link is unavailable.",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      const label = translationLabels[key];
      return typeof label === "function" ? label(values) : (label ?? key);
    },
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

import { WipQueueStatus } from "./wip-queue-status";

afterEach(cleanup);

describe("WipQueueStatus", () => {
  it("shows task units and links to the matching workflow card", () => {
    render(<WipQueueStatus taskId="queued-task" />);

    expect(screen.getByTestId("task-wip-queue-status")).toBeTruthy();
    expect(screen.getByText("Development / Review")).toBeTruthy();
    expect(screen.getByText("1 of 1 tasks waiting for this step.")).toBeTruthy();
    expect(screen.getByText("0 of 2 tasks admitted.")).toBeTruthy();
    expect(screen.getByTestId("wip-queue-settings-link").getAttribute("href")).toBe(
      `/settings/workspaces/${WORKSPACE_ID}/workflows#workflow-card-${WORKFLOW_ID}`,
    );
  });
});
