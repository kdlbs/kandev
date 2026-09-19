import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CreateGrantDialog } from "./create-grant-dialog";

const TASK_ID_INPUT = "grant-task-id-input";
const NOTE_INPUT = "grant-note-input";
const CREATE_GRANT_BUTTON = "create-grant-button";
const SUBMIT_BUTTON = "grant-create-submit";
const EXECUTE_CAPABILITY = "grant-cap-execute";
const SCOPE_ID_INPUT = "grant-scope-id-input";

const mocks = vi.hoisted(() => ({
  createWorkspaceCoordinatorGrant: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  createWorkspaceCoordinatorGrant: mocks.createWorkspaceCoordinatorGrant,
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}));

function renderDialog() {
  const onCreated = vi.fn();
  render(<CreateGrantDialog workspaceId="workspace-1" onCreated={onCreated} />);
  fireEvent.click(screen.getByTestId(CREATE_GRANT_BUTTON));
  return { onCreated };
}

function fillTaskId(taskId = "task-1") {
  fireEvent.change(screen.getByTestId(TASK_ID_INPUT), { target: { value: taskId } });
}

function selectCapability(testId = "grant-cap-orchestrate") {
  fireEvent.click(screen.getByTestId(testId));
}

function inputValue(testId: string) {
  return (screen.getByTestId(testId) as HTMLInputElement).value;
}

function checkboxState(testId: string) {
  return screen.getByTestId(testId).getAttribute("aria-checked");
}

describe("CreateGrantDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.createWorkspaceCoordinatorGrant.mockResolvedValue({ grant: { id: "grant-1" } });
  });

  afterEach(cleanup);

  it("rejects a missing coordinator task before calling the API", () => {
    renderDialog();
    selectCapability();

    fireEvent.click(screen.getByTestId(SUBMIT_BUTTON));

    expect(mocks.createWorkspaceCoordinatorGrant).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith("Coordinator task ID is required");
  });

  it("rejects a missing capability before calling the API", () => {
    renderDialog();
    fillTaskId();

    fireEvent.click(screen.getByTestId(SUBMIT_BUTTON));

    expect(mocks.createWorkspaceCoordinatorGrant).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith("Select at least one capability.");
  });

  it("shows localized provider failure feedback while preserving the open form", async () => {
    mocks.createWorkspaceCoordinatorGrant.mockRejectedValueOnce(new Error("conflict"));
    renderDialog();
    fillTaskId("task-kept");
    selectCapability("grant-cap-inspect");

    fireEvent.click(screen.getByTestId(SUBMIT_BUTTON));

    await waitFor(() => expect(mocks.createWorkspaceCoordinatorGrant).toHaveBeenCalledTimes(1));
    expect(mocks.toastError).toHaveBeenCalledWith("Failed to create grant. Try again.");
    expect(screen.getByRole("dialog", { name: "New Coordinator Grant" })).toBeTruthy();
    expect(inputValue(TASK_ID_INPUT)).toBe("task-kept");
    expect(checkboxState("grant-cap-inspect")).toBe("true");
  });

  it("closes, resets, and notifies the parent after a successful create", async () => {
    const { onCreated } = renderDialog();
    fillTaskId("task-reset");
    selectCapability(EXECUTE_CAPABILITY);
    fireEvent.change(screen.getByTestId(NOTE_INPUT), { target: { value: "temporary" } });

    fireEvent.click(screen.getByTestId(SUBMIT_BUTTON));

    await waitFor(() =>
      expect(mocks.createWorkspaceCoordinatorGrant).toHaveBeenCalledWith("workspace-1", {
        coordinator_task_id: "task-reset",
        scope_kind: "workspace",
        scope_id: undefined,
        capabilities: "execute",
        note: "temporary",
      }),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Grant created successfully.");
    expect(onCreated).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "New Coordinator Grant" })).toBeNull(),
    );

    fireEvent.click(screen.getByTestId(CREATE_GRANT_BUTTON));

    expect(inputValue(TASK_ID_INPUT)).toBe("");
    expect(inputValue(NOTE_INPUT)).toBe("");
    expect(checkboxState(EXECUTE_CAPABILITY)).toBe("false");
  });

  it("clears a workflow scope ID after a successful create", async () => {
    renderDialog();
    fillTaskId();
    selectCapability();
    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(screen.getByText("Workflow"));
    fireEvent.change(screen.getByTestId(SCOPE_ID_INPUT), { target: { value: "workflow-1" } });
    fireEvent.click(screen.getByTestId(SUBMIT_BUTTON));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "New Coordinator Grant" })).toBeNull(),
    );
    fireEvent.click(screen.getByTestId(CREATE_GRANT_BUTTON));
    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(screen.getByText("Workflow"));
    expect(inputValue(SCOPE_ID_INPUT)).toBe("");
  });
});
