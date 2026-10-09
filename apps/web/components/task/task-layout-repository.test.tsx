import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskLayout } from "./task-layout";

afterEach(cleanup);

// Every hop between the task page and the phone header is an optional prop, so
// a dropped one is not a type error. This asserts the hop TaskLayout owns: the
// repository label reaches the phone layout. `session-mobile-top-bar-repository`
// covers what the header does with it.
vi.mock("./mobile", () => ({
  SessionMobileLayout: function MobileLayout({
    repositoryLabel,
  }: {
    repositoryLabel?: string | null;
  }) {
    const [draft, setDraft] = useState("");
    return (
      <div data-testid="mobile-layout" data-repository-label={repositoryLabel ?? ""}>
        <input
          aria-label="Layout draft"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
      </div>
    );
  },
  SessionTabletLayout: () => <div data-testid="tablet-layout" />,
}));

vi.mock("./mobile/responsive-task-picker", () => ({
  ResponsiveTaskPicker: function Picker() {
    const [draft, setDraft] = useState("");
    return (
      <input
        aria-label="Picker draft"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
      />
    );
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true, usesDesktopWorkbench: false }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => true,
}));

vi.mock("./task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({ error: null, repositories: [] }),
}));

describe("TaskLayout repository label", () => {
  it("keeps layout state while task workspace metadata refreshes", () => {
    const view = render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />, {
      wrapper: StateProvider,
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Layout draft" }), {
      target: { value: "Keep this draft" },
    });
    view.rerender(<TaskLayout workspaceId={null} workflowId={null} />);
    view.rerender(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />);
    expect((screen.getByRole("textbox", { name: "Layout draft" }) as HTMLInputElement).value).toBe(
      "Keep this draft",
    );
  });

  it("retains picker drafts through missing workspace metadata and resets for a different workspace", () => {
    const view = render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />, {
      wrapper: StateProvider,
    });
    const draft = () => screen.getByRole("textbox", { name: "Picker draft" }) as HTMLInputElement;
    fireEvent.change(draft(), { target: { value: "Keep picker draft" } });
    view.rerender(<TaskLayout workspaceId={null} workflowId={null} />);
    expect(draft().value).toBe("Keep picker draft");
    view.rerender(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />);
    expect(draft().value).toBe("Keep picker draft");
    view.rerender(<TaskLayout workspaceId="ws-2" workflowId="wf-2" />);
    expect(draft().value).toBe("");
  });

  it("hands the repository label to the phone layout", () => {
    render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" repositoryLabel="kdlbs/kandev" />, {
      wrapper: StateProvider,
    });

    expect(screen.getByTestId("mobile-layout").getAttribute("data-repository-label")).toBe(
      "kdlbs/kandev",
    );
  });

  it("passes nothing along for a task with no repository", () => {
    render(<TaskLayout workspaceId="ws-1" workflowId="wf-1" />, { wrapper: StateProvider });

    expect(screen.getByTestId("mobile-layout").getAttribute("data-repository-label")).toBe("");
  });
});
