import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { importWorkflowsAction, previewWorkflowImportAction } from "@/app/actions/workspaces";
import type { WorkflowImportPreview, Workspace } from "@/lib/types/http";
import { ImportWorkflowsDialog } from "./workspace-workflows-dialogs";
import { WorkflowImportProfileSelection } from "./workflow-import-profile-selection";
import {
  reconcileWorkflowImportSelections,
  useWorkflowImport,
  workflowImportMissingSteps,
  workflowImportProfileLabel,
  workflowImportStepKey,
} from "./use-workflow-import";

vi.mock("@/app/actions/workspaces", () => ({
  importWorkflowsAction: vi.fn(),
  previewWorkflowImportAction: vi.fn(),
}));

const responsive = { isMobile: false, isFinePointer: true, usesTouchDrawer: false };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => responsive.usesTouchDrawer,
}));

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
  responsive.isFinePointer = true;
  responsive.usesTouchDrawer = false;
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

const workspace = { id: "workspace-1", name: "Workspace" } as Workspace;

function DeferredSubmitDialog() {
  const page = useWorkflowImport({
    workspace,
    router: { refresh: vi.fn() } as never,
    toast: vi.fn(),
  });
  return (
    <>
      <button
        type="button"
        data-testid="open-import"
        onClick={() => page.setIsImportDialogOpen(true)}
      >
        Open import
      </button>
      <ImportWorkflowsDialog
        open={page.isImportDialogOpen}
        onOpenChange={page.setIsImportDialogOpen}
        importYaml={page.importYaml}
        onImportYamlChange={page.setImportYaml}
        onFileUpload={page.handleFileUpload}
        fileInputRef={page.fileInputRef}
        onImport={page.handleImport}
        importLoading={page.importLoading}
        importPhase={page.importPhase}
        preview={page.preview}
        selections={page.selections}
        missingSteps={page.missingSteps}
        profileConflicts={page.profileConflicts}
        activeStepKey={page.activeStepKey}
        setActiveStepKey={page.setActiveStepKey}
        selectProfile={page.selectProfile}
        retryPreview={page.retryPreview}
      />
    </>
  );
}

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

it("uses the touch drawer for coarse-pointer tablet layouts", () => {
  responsive.isMobile = false;
  responsive.isFinePointer = false;
  responsive.usesTouchDrawer = true;

  render(
    <WorkflowImportProfileSelection
      open
      onOpenChange={vi.fn()}
      preview={preview}
      selections={{}}
      missingSteps={preview.steps}
      profileConflicts={[]}
      activeStepKey={"2:4"}
      onActiveStepKeyChange={vi.fn()}
      onSelectProfile={vi.fn()}
      onImport={vi.fn()}
      onRetryPreview={vi.fn()}
      importLoading={false}
    />,
  );

  expect(document.querySelector('[data-slot="drawer-content"]')).not.toBeNull();
  expect(screen.getByTestId("workflow-import-profile-select-2:4").className).toContain("min-h-11");
});

it("keeps the import action name stable and scopes busy status to one live region", () => {
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
      importPhase="submitting"
      preview={preview}
      selections={{ "2:4": preview.profiles[0].id }}
      missingSteps={[]}
      profileConflicts={[]}
      activeStepKey={null}
      setActiveStepKey={vi.fn()}
      selectProfile={vi.fn()}
      retryPreview={vi.fn()}
    />,
  );

  expect(screen.getByTestId("workflow-import-profile-submit").textContent).toContain(
    "Import workflows",
  );
  expect(screen.getByRole("status").textContent).toContain("Importing...");
  expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
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

it("keeps the rendered selection surface locked during a deferred final submit", async () => {
  let resolveImport!: (value: { created: string[]; skipped: string[] }) => void;
  vi.mocked(previewWorkflowImportAction).mockResolvedValueOnce(preview);
  vi.mocked(importWorkflowsAction).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveImport = resolve;
    }),
  );

  render(<DeferredSubmitDialog />);
  fireEvent.click(screen.getByTestId("open-import"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "portable yaml" } });
  fireEvent.click(screen.getByRole("button", { name: /^Import$/ }));

  await waitFor(() => expect(importWorkflowsAction).toHaveBeenCalledOnce());
  expect(screen.getByTestId("workflow-import-profile-selection")).toBeTruthy();
  expect(screen.queryByRole("textbox")).toBeNull();

  const submit = screen.getByTestId("workflow-import-profile-submit");
  expect(submit.hasAttribute("disabled")).toBe(true);
  fireEvent.click(submit);
  fireEvent.click(submit);
  expect(importWorkflowsAction).toHaveBeenCalledOnce();

  resolveImport({ created: ["Imported"], skipped: [] });
  await waitFor(() => expect(screen.queryByTestId("workflow-import-profile-selection")).toBeNull());
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
