import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ProposalSpec } from "@/lib/api/domains/coordinator-api";
import type { UseProposalEditOptionsResult } from "@/hooks/domains/coordinator/use-proposal-edit-options";

const editOptions = vi.hoisted(() => ({ current: undefined as unknown }));

vi.mock("@/hooks/domains/coordinator/use-proposal-edit-options", () => ({
  useProposalEditOptions: () => editOptions.current,
}));

import { EditForm } from "./edit-form";

afterEach(cleanup);

afterEach(() => {
  editOptions.current = undefined;
});

function spec(overrides: Partial<ProposalSpec> = {}): ProposalSpec {
  return {
    title: "Add tests",
    description: "Cover the new endpoint",
    rationale: "rationale",
    workflow_id: "wf-1",
    step_id: "step-1",
    repository_id: "repo-1",
    source_task_id: "t-1",
    ...overrides,
  };
}

function loadedOptions(
  overrides: Partial<UseProposalEditOptionsResult> = {},
): UseProposalEditOptionsResult {
  return {
    workflows: {
      status: "loaded",
      value: [{ id: "wf-1", workspace_id: "w-1", name: "Build" }] as never,
    },
    steps: {
      status: "loaded",
      value: [{ id: "step-1", name: "Review", is_start_step: true }] as never,
    },
    repositories: {
      status: "loaded",
      value: [{ id: "repo-1", workspace_id: "w-1", name: "app" }] as never,
    },
    retryWorkflows: vi.fn(),
    retryRepositories: vi.fn(),
    retrySteps: vi.fn(),
    snapshotWorkflowName: "Build",
    ...overrides,
  };
}

const APPROVE_BUTTON = "Approve with edits";

function approveButton() {
  return screen.getByRole("button", { name: APPROVE_BUTTON }) as HTMLButtonElement;
}

function renderForm(
  overrides: Partial<{
    spec: ProposalSpec;
    busy: boolean;
    serverError: { message: string; field: string | null } | null;
  }> = {},
) {
  return render(
    <EditForm
      workspaceId="w-1"
      spec={overrides.spec ?? spec()}
      busy={overrides.busy ?? false}
      serverError={overrides.serverError ?? null}
      onApprove={vi.fn()}
      onCancel={vi.fn()}
    />,
  );
}

describe("EditForm - title", () => {
  it("focuses the title field on mount", () => {
    editOptions.current = loadedOptions();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(document.activeElement).toBe(screen.getByLabelText("Title"));
  });

  it("blocks submit and shows an error for an empty title, without approving", () => {
    editOptions.current = loadedOptions();
    const onApprove = vi.fn();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={onApprove}
        onCancel={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: APPROVE_BUTTON }));
    expect(onApprove).not.toHaveBeenCalled();
    expect(screen.getByText("Title is required.")).not.toBeNull();
  });

  it("calls onCancel from the Cancel button", () => {
    editOptions.current = loadedOptions();
    const onCancel = vi.fn();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });
});

describe("EditForm - changed edits diff", () => {
  it("submits only the fields that changed from the original spec", () => {
    editOptions.current = loadedOptions();
    const onApprove = vi.fn();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={onApprove}
        onCancel={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Add more tests" } });
    fireEvent.click(screen.getByRole("button", { name: APPROVE_BUTTON }));
    expect(onApprove).toHaveBeenCalledWith({ title: "Add more tests" });
  });
});

describe("EditForm - workflow and step resolution", () => {
  it("notes when the proposed workflow was deleted", () => {
    editOptions.current = loadedOptions();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec({ workflow_id: "wf-deleted" })}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByText("The proposed workflow was deleted.")).not.toBeNull();
  });

  it("renders an empty step control with no messaging when the workflow itself is unresolved", () => {
    editOptions.current = loadedOptions();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec({ workflow_id: "wf-deleted" })}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.queryByText("Loading options")).toBeNull();
    expect(screen.queryByText("Could not load options")).toBeNull();
    expect(screen.queryByText("No step in this workflow can be entered manually.")).toBeNull();
  });

  it("shows a no-eligible-step message without disturbing the unchanged original step", () => {
    editOptions.current = loadedOptions({ steps: { status: "loaded", value: [] } });
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByText("No step in this workflow can be entered manually.")).not.toBeNull();
    expect(approveButton().disabled).toBe(true);
  });

  it("disables Approve with edits while the proposed workflow is deleted", () => {
    editOptions.current = loadedOptions();
    renderForm({ spec: spec({ workflow_id: "wf-deleted" }) });
    expect(approveButton().disabled).toBe(true);
  });

  it("disables Approve with edits while the proposed step is not eligible", () => {
    editOptions.current = loadedOptions();
    renderForm({ spec: spec({ step_id: "step-gone" }) });
    expect(screen.getByText("The proposed step is no longer eligible.")).not.toBeNull();
    expect(approveButton().disabled).toBe(true);
  });

  it("keeps the current workflow selected and shows its steps when the workflow list fails", () => {
    editOptions.current = loadedOptions({ workflows: { status: "error", value: [] } });
    renderForm();
    expect((screen.getByLabelText("Workflow") as HTMLInputElement).value).toBe("Build");
    expect(screen.getByLabelText("Step").textContent).toContain("Review");
    expect(approveButton().disabled).toBe(true);
  });
});

describe("EditForm - in-flight lock", () => {
  it("disables every option control and retry while a decision is in flight", () => {
    editOptions.current = loadedOptions({
      repositories: { status: "error", value: [] },
    });
    renderForm({ busy: true });
    for (const label of ["Workflow", "Step", "Repository"]) {
      expect((screen.getByLabelText(label) as HTMLButtonElement).disabled).toBe(true);
    }
    for (const retry of screen.getAllByRole("button", { name: "Try again" })) {
      expect((retry as HTMLButtonElement).disabled).toBe(true);
    }
  });

  it("disables the workflow and step retry buttons while busy", () => {
    editOptions.current = loadedOptions({
      workflows: { status: "error", value: [] },
      steps: { status: "error", value: [] },
    });
    renderForm({ busy: true });
    const retries = screen.getAllByRole("button", { name: "Try again" });
    expect(retries.length).toBe(2);
    for (const retry of retries) expect((retry as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("EditForm - loading and retry", () => {
  it("shows a loading placeholder while workflows are loading, and disables Approve", () => {
    editOptions.current = loadedOptions({ workflows: { status: "loading", value: [] } });
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getAllByDisplayValue("Loading options").length).toBeGreaterThan(0);
    expect(
      (screen.getByRole("button", { name: APPROVE_BUTTON }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("shows a retry control when workflows fail to load", () => {
    const retryWorkflows = vi.fn();
    editOptions.current = loadedOptions({
      workflows: { status: "error", value: [] },
      retryWorkflows,
    });
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Try again" })[0]);
    expect(retryWorkflows).toHaveBeenCalledOnce();
  });
});

describe("EditForm - repository", () => {
  it("notes when the current repository is no longer in the list, without blocking Approve", () => {
    editOptions.current = loadedOptions();
    const { container } = render(
      <EditForm
        workspaceId="w-1"
        spec={spec({ repository_id: "repo-deleted" })}
        busy={false}
        serverError={null}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const values = container.querySelectorAll('[data-slot="select-value"]');
    expect(values[values.length - 1]?.textContent).toBe("Current repository (unavailable)");
    expect(
      (screen.getByRole("button", { name: APPROVE_BUTTON }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});

describe("EditForm - server error focus", () => {
  it.each([
    ["workflow_id", "Workflow"],
    ["step_id", "Step"],
    ["repository_id", "Repository"],
    ["description", "Description"],
  ])("moves focus to the %s field and renders its error as an alert", (field, label) => {
    editOptions.current = loadedOptions();
    renderForm({ serverError: { message: "Not allowed here", field } });
    expect(document.activeElement).toBe(screen.getByLabelText(label));
    expect(screen.getByRole("alert").textContent).toBe("Not allowed here");
  });

  it("shows an error for a field not on the form above the buttons and focuses it", () => {
    editOptions.current = loadedOptions();
    renderForm({ serverError: { message: "Rationale is too long", field: "rationale" } });
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("Rationale is too long");
    expect(document.activeElement).toBe(alert);
  });
});

describe("EditForm - server errors", () => {
  it("shows a server error next to its named field", () => {
    editOptions.current = loadedOptions();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={{ message: "That repository is archived", field: "repository_id" }}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByText("That repository is archived")).not.toBeNull();
  });

  it("shows a general server error above the buttons when no field is named", () => {
    editOptions.current = loadedOptions();
    render(
      <EditForm
        workspaceId="w-1"
        spec={spec()}
        busy={false}
        serverError={{ message: "Could not reach Kandev.", field: null }}
        onApprove={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("Could not reach Kandev.");
    expect(document.activeElement).toBe(alert);
  });
});
