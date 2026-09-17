import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkflowImportPreview } from "@/lib/types/http";
import { ImportWorkflowsDialog } from "./workspace-workflows-dialogs";
import { WorkflowImportProfileSelection } from "./workflow-import-profile-selection";
import {
  reconcileWorkflowImportSelections,
  workflowImportMissingSteps,
  workflowImportProfileLabel,
  workflowImportStepKey,
} from "./use-workflow-import";

const responsive = { isMobile: false };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
});

const preview: WorkflowImportPreview = {
  skipped: [],
  profiles: [
    {
      id: "profile-1",
      name: "Local Codex",
      agent_name: "Codex",
      model: "gpt-5",
      mode: "full",
      updated_at: "2026-09-17T13:00:00Z",
    },
  ],
  steps: [
    {
      workflow_index: 2,
      workflow_name: "Imported",
      step_position: 4,
      step_name: "Implement",
      requested_profile: { agent_name: "Codex", model: "gpt-5", mode: "full" },
      matched_profile: { id: "profile-1", updated_at: "2026-09-17T13:00:00Z" },
    },
  ],
};

it("uses workflow index and step position as an independent key", () => {
  expect(workflowImportStepKey(preview.steps[0])).toBe("2:4");
  expect(workflowImportMissingSteps(preview, {})).toEqual(preview.steps);
});

it("includes all profile identity fields in the searchable label", () => {
  expect(workflowImportProfileLabel(preview.profiles[0])).toContain("Local Codex");
  expect(workflowImportProfileLabel(preview.profiles[0])).toContain("Codex");
  expect(workflowImportProfileLabel(preview.profiles[0])).toContain("gpt-5");
  expect(workflowImportProfileLabel(preview.profiles[0])).toContain("full");
});

it("retains a selected profile only when its revision is unchanged", () => {
  const selected = { "2:4": "profile-1" };
  expect(reconcileWorkflowImportSelections(preview, preview, selected)).toEqual(selected);

  const changed = {
    ...preview,
    profiles: [{ ...preview.profiles[0], updated_at: "2026-09-17T14:00:00Z" }],
  };
  expect(reconcileWorkflowImportSelections(preview, changed, selected)).toEqual({});
});

it("renders settings access and retry for an empty eligible catalog on desktop and phone", () => {
  const emptyPreview: WorkflowImportPreview = {
    skipped: [],
    profiles: [],
    steps: [
      {
        workflow_index: 0,
        workflow_name: "Imported",
        step_position: 1,
        step_name: "Implement",
        requested_profile: { agent_name: "Missing", model: "model", mode: "mode" },
      },
    ],
  };
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    preview: emptyPreview,
    selections: {},
    missingSteps: emptyPreview.steps,
    profileConflicts: [],
    activeStepKey: "0:1",
    onActiveStepKeyChange: vi.fn(),
    onSelectProfile: vi.fn(),
    onImport: vi.fn(),
    onRetryPreview: vi.fn(),
    importLoading: false,
  };

  for (const isMobile of [false, true]) {
    responsive.isMobile = isMobile;
    render(<WorkflowImportProfileSelection {...props} />);

    const emptyState = screen.getByTestId("workflow-import-no-profiles");
    expect(emptyState).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open agent profile settings" })).toBeTruthy();
    expect(within(emptyState).getByRole("button", { name: "Retry preview" })).toBeTruthy();
    cleanup();
  }
});

it("keeps the selection surface mounted and draft controls unavailable while submitting", () => {
  const submittingPreview: WorkflowImportPreview = {
    ...preview,
    steps: preview.steps.map((step) => ({
      ...step,
      matched_profile: {
        id: preview.profiles[0].id,
        updated_at: preview.profiles[0].updated_at,
      },
    })),
  };
  const onImport = vi.fn();

  for (const isMobile of [false, true]) {
    responsive.isMobile = isMobile;
    render(
      <ImportWorkflowsDialog
        open
        onOpenChange={vi.fn()}
        importYaml="portable yaml"
        onImportYamlChange={vi.fn()}
        onFileUpload={vi.fn()}
        fileInputRef={{ current: null }}
        onImport={onImport}
        importLoading
        importPhase="submitting"
        preview={submittingPreview}
        selections={{ "0:0": preview.profiles[0].id, "0:1": preview.profiles[0].id }}
        missingSteps={[]}
        profileConflicts={[]}
        activeStepKey={null}
        setActiveStepKey={vi.fn()}
        selectProfile={vi.fn()}
        retryPreview={vi.fn()}
      />,
    );

    expect(screen.getByTestId("workflow-import-profile-selection")).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();

    const submit = screen.getByTestId("workflow-import-profile-submit");
    expect(submit.hasAttribute("disabled")).toBe(true);
    fireEvent.click(submit);
    fireEvent.click(submit);
    cleanup();
  }
  expect(onImport).not.toHaveBeenCalled();
});

it("disables draft inputs while the initial preview is pending", () => {
  render(
    <ImportWorkflowsDialog
      open
      onOpenChange={vi.fn()}
      importYaml="portable yaml"
      onImportYamlChange={vi.fn()}
      onFileUpload={vi.fn()}
      fileInputRef={{ current: null }}
      onImport={vi.fn()}
      importLoading
      importPhase="previewing"
      preview={null}
      selections={{}}
      missingSteps={[]}
      profileConflicts={[]}
      activeStepKey={null}
      setActiveStepKey={vi.fn()}
      selectProfile={vi.fn()}
      retryPreview={vi.fn()}
    />,
  );

  expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(true);
  expect((document.querySelector('input[type="file"]') as HTMLInputElement).disabled).toBe(true);
});
