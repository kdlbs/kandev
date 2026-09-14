import { cleanup, fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  WorkflowSyncController,
  WorkflowSyncFormState,
} from "@/hooks/domains/settings/use-workflow-sync";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";
import { WorkflowSyncDialog } from "./workflow-sync-dialog";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

const WORKFLOWS_DIR = ".kandev/workflows";
const REMOVE_TEST_ID = "workflow-sync-remove";
const REMOVE_CONFIRMATION_TEST_ID = "workflow-sync-remove-confirmation";
const REMOVE_CONFIRM_TEST_ID = "workflow-sync-remove-confirm";

const EMPTY_FORM: WorkflowSyncFormState = {
  provider: "github",
  repo_owner: "",
  repo_name: "",
  project_path: "",
  branch: "main",
  path: WORKFLOWS_DIR,
  interval_seconds: 300,
  poll_enabled: true,
};

function config(overrides: Partial<WorkflowSyncConfig> = {}): WorkflowSyncConfig {
  return {
    workspace_id: "workspace-1",
    provider: "github",
    repo_owner: "acme",
    repo_name: "flows",
    project_path: "",
    branch: "main",
    path: WORKFLOWS_DIR,
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    ...overrides,
  };
}

function formForConfig(currentConfig: WorkflowSyncConfig | null): WorkflowSyncFormState {
  if (!currentConfig) return { ...EMPTY_FORM };
  return {
    provider: currentConfig.provider,
    repo_owner: currentConfig.repo_owner,
    repo_name: currentConfig.repo_name,
    project_path: currentConfig.project_path,
    branch: currentConfig.branch,
    path: currentConfig.path,
    interval_seconds: currentConfig.interval_seconds,
    poll_enabled: currentConfig.poll_enabled,
  };
}

function controller(overrides: Partial<WorkflowSyncController> = {}): WorkflowSyncController {
  const currentConfig = overrides.config === undefined ? config() : overrides.config;
  return {
    config: currentConfig,
    form: formForConfig(currentConfig),
    url: currentConfig ? "https://github.com/acme/flows" : "",
    urlInvalid: false,
    loading: false,
    saving: false,
    syncing: false,
    update: vi.fn(),
    setUrlInput: vi.fn(),
    setProvider: vi.fn(),
    handleSave: vi.fn().mockResolvedValue(true),
    handleDelete: vi.fn().mockResolvedValue(true),
    handleSyncNow: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("WorkflowSyncDialog phone removal confirmation", () => {
  it("does not dismiss a replacement dialog when an unmounted removal completes", async () => {
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    let finish!: (result: boolean) => void;
    const sync = controller({
      handleDelete: vi.fn(
        () =>
          new Promise<boolean>((resolve) => {
            finish = resolve;
          }),
      ),
    });
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <WorkflowSyncDialog key="original" open onOpenChange={onOpenChange} sync={sync} />,
    );
    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    fireEvent.click(screen.getByTestId(REMOVE_CONFIRM_TEST_ID));
    await waitFor(() => expect(sync.handleDelete).toHaveBeenCalledTimes(1));
    rerender(
      <WorkflowSyncDialog key="replacement" open onOpenChange={onOpenChange} sync={controller()} />,
    );
    await act(async () => finish(true));
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByTestId(REMOVE_TEST_ID)).toBeTruthy();
  });

  it("uses one phone dialog, preserves its draft, and retries a failed removal", async () => {
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    let finish!: (result: boolean) => void;
    const sync = controller({
      handleDelete: vi
        .fn()
        .mockImplementationOnce(
          () =>
            new Promise<boolean>((resolve) => {
              finish = resolve;
            }),
        )
        .mockResolvedValue(true),
    });
    const onOpenChange = vi.fn();
    render(<WorkflowSyncDialog open onOpenChange={onOpenChange} sync={sync} />);
    const dialog = screen.getByTestId("workflow-sync-dialog");
    const originalId = dialog.id;
    const input = screen.getByTestId("workflow-sync-branch-input");
    const trigger = screen.getByTestId(REMOVE_TEST_ID);
    fireEvent.click(trigger);
    expect(screen.getByRole("button", { name: "Back" })).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(dialog.id).toBe(originalId);
    expect(input.closest("[inert]")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(screen.getByTestId("workflow-sync-branch-input")).toBe(input);
    expect(sync.handleDelete).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    const confirm = screen.getByTestId(REMOVE_CONFIRM_TEST_ID);
    fireEvent.click(confirm);
    fireEvent.click(confirm);
    await waitFor(() => expect(sync.handleDelete).toHaveBeenCalledTimes(1));
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    await act(async () => finish(false));
    await waitFor(() =>
      expect((screen.getByTestId(REMOVE_CONFIRM_TEST_ID) as HTMLButtonElement).disabled).toBe(
        false,
      ),
    );
    expect(onOpenChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId(REMOVE_CONFIRM_TEST_ID));
    await waitFor(() => expect(sync.handleDelete).toHaveBeenCalledTimes(2));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("WorkflowSyncDialog wider removal confirmation", () => {
  it("keeps removal inline in the existing dialog without a second overlay", () => {
    const sync = controller();
    render(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));

    expect(screen.getByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(screen.getByTestId(REMOVE_CONFIRM_TEST_ID)).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(sync.handleDelete).not.toHaveBeenCalled();
  });

  it("cancels without mutating sync state", () => {
    const sync = controller();
    const onOpenChange = vi.fn();
    render(<WorkflowSyncDialog open onOpenChange={onOpenChange} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(sync.handleDelete).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.queryByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeNull();
    expect(screen.getByTestId(REMOVE_TEST_ID)).toBeTruthy();
  });

  it("prevents saves while removal confirmation is active", () => {
    const sync = controller();
    render(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));

    const save = screen.getByTestId<HTMLButtonElement>("workflow-sync-save");
    expect(save.disabled).toBe(true);
    fireEvent.click(save);
    expect(sync.handleSave).not.toHaveBeenCalled();
  });

  it("confirms removal once and closes after successful mutation", async () => {
    const sync = controller();
    const onOpenChange = vi.fn();
    render(<WorkflowSyncDialog open onOpenChange={onOpenChange} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    fireEvent.click(screen.getByTestId(REMOVE_CONFIRM_TEST_ID));

    await waitFor(() => expect(sync.handleDelete).toHaveBeenCalledTimes(1));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps inline actions available when removal fails", async () => {
    const sync = controller({ handleDelete: vi.fn().mockResolvedValue(false) });
    const onOpenChange = vi.fn();
    render(<WorkflowSyncDialog open onOpenChange={onOpenChange} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    fireEvent.click(screen.getByTestId(REMOVE_CONFIRM_TEST_ID));

    await waitFor(() => expect(sync.handleDelete).toHaveBeenCalledTimes(1));
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeTruthy();
  });

  it("clears inline state when the dialog closes or target changes", async () => {
    const sync = controller();
    const { rerender } = render(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    expect(screen.getByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeTruthy();

    rerender(<WorkflowSyncDialog open={false} onOpenChange={vi.fn()} sync={sync} />);
    rerender(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={sync} />);
    expect(screen.queryByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeNull();

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    const nextSync = controller({ config: config({ repo_name: "new-flows" }) });
    rerender(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={nextSync} />);
    await waitFor(() => expect(screen.queryByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeNull());
  });

  it("clears inline state when the user edits the repository URL", async () => {
    const sync = controller();
    const { rerender } = render(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={sync} />);

    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    fireEvent.change(screen.getByTestId("workflow-sync-url-input"), {
      target: { value: "https://github.com/acme/other-flows" },
    });
    expect(sync.setUrlInput).toHaveBeenCalledWith("https://github.com/acme/other-flows");

    const editedSync = controller({
      form: { ...sync.form, repo_name: "other-flows" },
      url: "https://github.com/acme/other-flows",
    });
    rerender(<WorkflowSyncDialog open onOpenChange={vi.fn()} sync={editedSync} />);

    await waitFor(() => expect(screen.queryByTestId(REMOVE_CONFIRMATION_TEST_ID)).toBeNull());
    expect(screen.getByTestId(REMOVE_TEST_ID)).toBeTruthy();
  });
});
