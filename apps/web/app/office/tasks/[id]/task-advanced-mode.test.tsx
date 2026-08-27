import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { Task } from "./types";
import {
  OfficeTopbarChromeProvider,
  useOfficeTopbarChrome,
} from "../../components/office-topbar-context";

vi.mock("@/lib/routing/client-dynamic", () => ({
  default: () =>
    function MockOfficeDockviewLayout() {
      return null;
    },
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("./status-icon", () => ({
  StatusIcon: () => null,
}));

vi.mock("./use-advanced-session", () => ({
  useAdvancedSession: () => ({ sessionId: null, isSessionEnded: false }),
}));

vi.mock("../../components/execution-indicator", () => ({
  ExecutionIndicator: ({ status }: { status: string }) => (
    <div data-testid="execution-indicator">{status}</div>
  ),
}));

vi.mock("@kandev/ui/label", () => ({
  Label: ({ children }: { children: React.ReactNode }) => <label>{children}</label>,
}));

vi.mock("@kandev/ui/switch", () => ({
  Switch: () => null,
}));

import { TaskAdvancedMode } from "./task-advanced-mode";

const baseTask: Task = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "E2E-1",
  title: "Advanced mode task",
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "",
  createdAt: "2026-05-01T10:00:00Z",
  updatedAt: "2026-05-01T10:00:00Z",
};

afterEach(() => cleanup());

function TopbarActions() {
  const chrome = useOfficeTopbarChrome();
  return <>{chrome?.actions}</>;
}

function renderAdvanced(task: Task) {
  render(
    <OfficeTopbarChromeProvider>
      <TaskAdvancedMode task={task} onToggleSimple={() => {}} />
      <TopbarActions />
    </OfficeTopbarChromeProvider>,
  );
}

describe("TaskAdvancedMode ExecutionIndicator wiring", () => {
  // Regression: the detail page's store-seeded task must feed ExecutionIndicator
  // the raw backend status, not the normalized canonical one, so it still tells
  // SCHEDULING (live) apart from WAITING_FOR_INPUT (not live) — the same
  // distinction task-row.tsx already preserves for the board/list views.
  it("prefers rawStatus over status so a SCHEDULING task still shows Live", () => {
    renderAdvanced({ ...baseTask, status: "todo", rawStatus: "SCHEDULING" } as Task);
    expect(screen.getByTestId("execution-indicator").textContent).toBe("SCHEDULING");
  });

  it("prefers rawStatus over status so a WAITING_FOR_INPUT task does not falsely show Live", () => {
    renderAdvanced({ ...baseTask, status: "in_progress", rawStatus: "WAITING_FOR_INPUT" } as Task);
    expect(screen.getByTestId("execution-indicator").textContent).toBe("WAITING_FOR_INPUT");
  });

  it("falls back to status when rawStatus is absent", () => {
    renderAdvanced({ ...baseTask, status: "in_progress" });
    expect(screen.getByTestId("execution-indicator").textContent).toBe("in_progress");
  });
});
