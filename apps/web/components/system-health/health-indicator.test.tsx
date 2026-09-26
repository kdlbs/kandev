import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { HealthIssuesDialog } from "./health-indicator";
import type { HealthIssue } from "@/lib/types/health";

const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => router,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ workspaces: { activeId: "workspace-1" } }),
}));

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function issue(overrides: Partial<HealthIssue> = {}): HealthIssue {
  return {
    id: "disk-full",
    category: "storage",
    title: "Disk space is low",
    message: "Free some space before starting another task.",
    severity: "warning",
    fix_url: "/settings/system/status",
    fix_label: "View system status",
    ...overrides,
  };
}

function renderDialog(issues: HealthIssue[] = [issue()]) {
  return render(<HealthIssuesDialog open onOpenChange={vi.fn()} issues={issues} />);
}

describe("HealthIssuesDialog", () => {
  it("keeps issue Fix navigation unchanged", () => {
    const onOpenChange = vi.fn();
    render(<HealthIssuesDialog open onOpenChange={onOpenChange} issues={[issue()]} />);

    fireEvent.click(screen.getByRole("button", { name: "View system status" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(router.push).toHaveBeenCalledWith("/settings/system/status");
  });

  it("exposes one scroll body for issue cards while keeping the dialog shell local", () => {
    renderDialog([
      issue({ id: "disk-full" }),
      issue({ id: "git-missing", title: "Git is missing" }),
    ]);

    const dialog = screen.getByTestId("system-health-issues-dialog");
    const body = screen.getByTestId("system-health-issues-body");

    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(dialog.contains(body)).toBe(true);
    expect(body.textContent).toContain("Disk space is low");
  });
});
