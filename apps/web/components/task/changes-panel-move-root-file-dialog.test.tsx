import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceRootDestination } from "@/lib/types/workspace-files";
import { ChangesPanelMoveRootFileDialog } from "./changes-panel-move-root-file-dialog";

const mocks = vi.hoisted(() => ({
  renameFile: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({}),
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  renameFile: mocks.renameFile,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

const destinations: WorkspaceRootDestination[] = [
  { repository_name: "frontend", path: "apps/frontend" },
  { repository_name: "backend", path: "apps/backend" },
];

function renderDialog(onOpenChange = vi.fn()) {
  return render(
    <ChangesPanelMoveRootFileDialog
      open
      onOpenChange={onOpenChange}
      sessionId="session-1"
      path="dashboard.json"
      destinations={destinations}
    />,
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  mocks.renameFile.mockReset();
  mocks.toast.mockReset();
});

afterEach(cleanup);

describe("ChangesPanelMoveRootFileDialog", () => {
  it("has no default destination and disables Move", () => {
    renderDialog();

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByRole("radio", { name: /frontend/ }).getAttribute("aria-checked")).toBe(
      "false",
    );
    expect((screen.getByRole("button", { name: "Move" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("moves to the selected checkout without a repo scope", async () => {
    const onOpenChange = vi.fn();
    mocks.renameFile.mockResolvedValue({ success: true });
    renderDialog(onOpenChange);

    fireEvent.click(screen.getByRole("radio", { name: /frontend/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(mocks.renameFile).toHaveBeenCalledOnce());
    expect(mocks.renameFile).toHaveBeenCalledWith(
      expect.anything(),
      "session-1",
      "dashboard.json",
      "apps/frontend/dashboard.json",
    );
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps the dialog open and shows an error toast when the target exists", async () => {
    mocks.renameFile.mockRejectedValue(new Error("destination already exists"));
    renderDialog();

    fireEvent.click(screen.getByRole("radio", { name: /backend/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledOnce());
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Could not move file",
        description: "destination already exists",
        variant: "error",
      }),
    );
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("ignores dismissal while a move is pending", async () => {
    const response = deferred<{ success: boolean }>();
    const onOpenChange = vi.fn();
    mocks.renameFile.mockReturnValue(response.promise);
    renderDialog(onOpenChange);

    fireEvent.click(screen.getByRole("radio", { name: /backend/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));
    await waitFor(() => expect(mocks.renameFile).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    response.resolve({ success: true });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});
