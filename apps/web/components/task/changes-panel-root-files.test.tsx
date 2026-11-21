import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";
import { ChangesPanelRootFiles } from "./changes-panel-root-files";

const listing: WorkspaceRootFilesResponse = {
  supported: true,
  files: [{ path: "dashboard.json", size: 12, modified_at: "2026-11-19T10:00:00Z" }],
  truncated: false,
  destinations: [{ repository_name: "web", path: "apps/web" }],
};

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

afterEach(cleanup);

describe("ChangesPanelRootFiles", () => {
  it("is hidden when root files are unsupported or empty", () => {
    const { rerender } = render(
      <ChangesPanelRootFiles
        listing={{ ...listing, supported: false }}
        sessionId="session-1"
        onEditFile={vi.fn()}
      />,
    );
    expect(screen.queryByTestId("changes-workspace-root-files")).toBeNull();

    rerender(
      <ChangesPanelRootFiles
        listing={{ ...listing, files: [] }}
        sessionId="session-1"
        onEditFile={vi.fn()}
      />,
    );
    expect(screen.queryByTestId("changes-workspace-root-files")).toBeNull();
  });

  it("shows root-relative files, the truncation note, and opens without a repository scope", () => {
    const onEditFile = vi.fn();
    render(
      <ChangesPanelRootFiles
        listing={{ ...listing, truncated: true }}
        sessionId="session-1"
        onEditFile={onEditFile}
      />,
    );

    expect(screen.getByRole("heading", { name: "Outside repositories" })).toBeTruthy();
    expect(screen.getByTestId("changes-workspace-root-files").textContent).toContain("(1)");
    expect(screen.getByText("Some workspace root files are not shown.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "dashboard.json" }));
    expect(onEditFile).toHaveBeenCalledWith("dashboard.json", undefined);
  });

  it("offers the move action when a destination repository is available", async () => {
    render(<ChangesPanelRootFiles listing={listing} sessionId="session-1" onEditFile={vi.fn()} />);

    fireEvent.pointerDown(screen.getByRole("button", { name: "Show more actions" }), {
      button: 0,
      ctrlKey: false,
    });
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: "Move to repository..." })).toBeTruthy();
    });
  });

  it("closes the move dialog when the session changes", async () => {
    const { rerender } = render(
      <ChangesPanelRootFiles listing={listing} sessionId="session-1" onEditFile={vi.fn()} />,
    );

    fireEvent.pointerDown(screen.getByRole("button", { name: "Show more actions" }), {
      button: 0,
      ctrlKey: false,
    });
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Move to repository..." })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Move to repository..." }));
    expect(screen.getByRole("dialog")).toBeTruthy();

    rerender(
      <ChangesPanelRootFiles listing={listing} sessionId="session-2" onEditFile={vi.fn()} />,
    );

    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
