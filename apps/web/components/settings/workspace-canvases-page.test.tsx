import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Canvas, CanvasListResponse } from "@/lib/api/domains/canvas-api";

const mocks = vi.hoisted(() => ({
  listWorkspaceCanvases: vi.fn(),
  archiveCanvas: vi.fn(),
  restoreCanvas: vi.fn(),
  removeCanvas: vi.fn(),
  startCanvasEdit: vi.fn(),
  push: vi.fn(),
}));

const COPY = {
  workspace: "Workspace",
  workspaceCanvases: "Workspace canvases",
  workspaceDescription: "Manage the canvases available in {{workspace}}.",
  refresh: "Refresh",
  loadingCanvases: "Loading workspace canvases",
  noWorkspaceCanvases: "No workspace canvases yet.",
  activeCanvases: "Active canvases",
  archivedCanvases: "Archived canvases",
  active: "Active",
  archived: "Archived",
  openCanvas: "Open canvas",
  editCanvas: "Edit canvas",
  archiveCanvas: "Archive",
  restoreCanvas: "Restore",
  removeCanvas: "Remove",
  removeDescription: "Remove {{title}} and its canvas releases? This action cannot be undone.",
  cancel: "Cancel",
  loadFailed: "Failed to load the canvas.",
} as const;

const REMOVE_ROW_TEST_ID = "workspace-canvas-remove-canvas";

const TRANSLATIONS: Record<string, string> = {
  "common:workspace": COPY.workspace,
  "common:cancel": COPY.cancel,
  "canvases:workspaceCanvases": COPY.workspaceCanvases,
  "canvases:workspaceCanvasesDescription": COPY.workspaceDescription,
  "canvases:refresh": COPY.refresh,
  "canvases:loadingCanvases": COPY.loadingCanvases,
  "canvases:noWorkspaceCanvases": COPY.noWorkspaceCanvases,
  "canvases:activeCanvases": COPY.activeCanvases,
  "canvases:archivedCanvases": COPY.archivedCanvases,
  "canvases:statusActive": COPY.active,
  "canvases:statusArchived": COPY.archived,
  "canvases:statusPending": "Pending",
  "canvases:statusDisabled": "Disabled",
  "canvases:statusError": "Error",
  "canvases:openCanvas": COPY.openCanvas,
  "canvases:editCanvas": COPY.editCanvas,
  "canvases:permissionsAndReleases": "Permissions and releases",
  "canvases:archiveCanvas": COPY.archiveCanvas,
  "canvases:restoreCanvas": COPY.restoreCanvas,
  "canvases:removeCanvas": COPY.removeCanvas,
  "canvases:removeCanvasDescription": COPY.removeDescription,
  "canvases:loadFailed": COPY.loadFailed,
};

const translate = (key: string, options?: Record<string, unknown>) =>
  (TRANSLATIONS[key] ?? key).replace(/\{\{(\w+)\}\}/g, (_, name: string) => {
    return String(options?.[name] ?? `{{${name}}}`);
  });

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: translate }),
}));

vi.mock("@/lib/api/domains/canvas-api", () => ({
  archiveCanvas: mocks.archiveCanvas,
  canvasHref: (canvasId: string) => `/canvases/${encodeURIComponent(canvasId)}`,
  listWorkspaceCanvases: mocks.listWorkspaceCanvases,
  removeCanvas: mocks.removeCanvas,
  restoreCanvas: mocks.restoreCanvas,
  startCanvasEdit: mocks.startCanvasEdit,
}));

vi.mock("@/components/settings/canvas-lifecycle-dialogs", () => ({
  CanvasReleaseDialog: () => null,
}));

const storeState = {
  workspaces: {
    items: [
      { id: "workspace-1", name: "Design workspace" },
      { id: "workspace-2", name: "Review workspace" },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("@/components/canvas/canvas-task-create-launcher", () => ({
  CanvasTaskCreateLauncher: ({ workspaceId }: { workspaceId: string }) => (
    <button type="button" data-testid="settings-create-canvas" data-workspace-id={workspaceId}>
      Create canvas
    </button>
  ),
}));

import { WorkspaceCanvasesPage } from "./workspace-canvases-page";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function canvas(overrides: Partial<Canvas> = {}): Canvas {
  return {
    id: "canvas-1",
    plugin_instance_id: "instance-1",
    plugin_id: "plugin-1",
    workspace_id: "workspace-1",
    scope_kind: "workspace",
    title: "Design canvas",
    status: "active",
    active_release_id: "release-1",
    ...overrides,
  };
}

function response(canvases: Canvas[]): CanvasListResponse {
  return { canvases };
}

beforeEach(() => {
  mocks.listWorkspaceCanvases.mockReset().mockResolvedValue(response([]));
  mocks.archiveCanvas.mockReset();
  mocks.restoreCanvas.mockReset();
  mocks.removeCanvas.mockReset();
  mocks.startCanvasEdit.mockReset();
  mocks.push.mockReset();
});

afterEach(cleanup);

describe("WorkspaceCanvasesPage", () => {
  it("offers the shared canvas task launcher for the selected workspace", async () => {
    render(<WorkspaceCanvasesPage workspaceId="workspace-2" />);

    expect(await screen.findByTestId("settings-create-canvas")).toBeTruthy();
    expect(screen.getByTestId("settings-create-canvas").getAttribute("data-workspace-id")).toBe(
      "workspace-2",
    );
  });

  it("shows the loading state first and renders active and archived canvases", async () => {
    const request = deferred<CanvasListResponse>();
    mocks.listWorkspaceCanvases.mockReturnValue(request.promise);

    render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);

    expect(screen.getByTestId("workspace-canvases-page")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe(COPY.loadingCanvases);
    expect((screen.getByRole("button", { name: COPY.refresh }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(mocks.listWorkspaceCanvases).toHaveBeenCalledWith("workspace-1", {
      includeArchived: true,
    });

    request.resolve(
      response([
        canvas({ id: "active-canvas", title: "Live canvas" }),
        canvas({
          id: "archived-canvas",
          title: "Retained canvas",
          status: "archived",
        }),
        canvas({ id: "removed-canvas", title: "Removed canvas", status: "removed" }),
      ]),
    );

    await waitFor(() => expect(screen.getByText("Live canvas")).toBeTruthy());
    expect(screen.getByText(COPY.activeCanvases)).toBeTruthy();
    expect(screen.getByText(COPY.archivedCanvases)).toBeTruthy();
    expect(screen.getByText("Retained canvas")).toBeTruthy();
    expect(screen.queryByText("Removed canvas")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("ignores a stale reload response after a newer reload completes", async () => {
    const initial = canvas({ id: "initial-canvas", title: "Initial canvas" });
    const stale = deferred<CanvasListResponse>();
    const fresh = deferred<CanvasListResponse>();
    mocks.listWorkspaceCanvases
      .mockResolvedValueOnce(response([initial]))
      .mockReturnValueOnce(stale.promise)
      .mockReturnValueOnce(fresh.promise);

    const { rerender } = render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);
    await waitFor(() => expect(screen.getByText("Initial canvas")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: COPY.refresh }));
    await waitFor(() => expect(mocks.listWorkspaceCanvases).toHaveBeenCalledTimes(2));

    rerender(<WorkspaceCanvasesPage workspaceId="workspace-2" />);
    await waitFor(() => expect(mocks.listWorkspaceCanvases).toHaveBeenCalledTimes(3));

    const current = canvas({
      id: "current-canvas",
      title: "Current canvas",
      workspace_id: "workspace-2",
    });
    fresh.resolve(response([current]));
    await waitFor(() => expect(screen.getByText("Current canvas")).toBeTruthy());

    await act(async () => {
      stale.resolve(response([canvas({ id: "stale-canvas", title: "Stale canvas" })]));
      await stale.promise;
    });
    expect(screen.queryByText("Stale canvas")).toBeNull();
    expect(screen.getByText("Current canvas")).toBeTruthy();
  });
});

describe("WorkspaceCanvasesPage lifecycle", () => {
  it("updates archive and restore state in place without reloading the list", async () => {
    const activeCanvas = canvas({ id: "lifecycle-canvas", title: "Lifecycle canvas" });
    const archived = { ...activeCanvas, status: "archived" as const };
    mocks.listWorkspaceCanvases.mockResolvedValue(response([activeCanvas]));
    mocks.archiveCanvas.mockResolvedValue(archived);
    mocks.restoreCanvas.mockResolvedValue(activeCanvas);

    render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);
    const row = await screen.findByTestId("workspace-canvas-lifecycle-canvas");

    fireEvent.click(within(row).getByRole("button", { name: COPY.archiveCanvas }));
    await waitFor(() => expect(mocks.archiveCanvas).toHaveBeenCalledWith(activeCanvas.id));
    expect(screen.queryByText(COPY.activeCanvases)).toBeNull();
    expect(screen.getByText(COPY.archivedCanvases)).toBeTruthy();
    expect(screen.getByRole("button", { name: COPY.restoreCanvas })).toBeTruthy();
    expect(
      (
        within(screen.getByTestId("workspace-canvas-lifecycle-canvas")).getByRole("button", {
          name: COPY.editCanvas,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: COPY.restoreCanvas }));
    await waitFor(() => expect(mocks.restoreCanvas).toHaveBeenCalledWith(activeCanvas.id));
    expect(screen.getByText(COPY.activeCanvases)).toBeTruthy();
    expect(screen.queryByText(COPY.archivedCanvases)).toBeNull();
    expect(screen.getByRole("button", { name: COPY.archiveCanvas })).toBeTruthy();
    expect(mocks.listWorkspaceCanvases).toHaveBeenCalledTimes(1);
  });

  it("cancels removal without mutating, then confirms and removes the canvas", async () => {
    const target = canvas({ id: "remove-canvas", title: "Canvas to remove" });
    mocks.listWorkspaceCanvases.mockResolvedValue(response([target]));
    mocks.removeCanvas.mockResolvedValue(undefined);

    render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);
    const row = await screen.findByTestId(REMOVE_ROW_TEST_ID);

    fireEvent.click(within(row).getByRole("button", { name: COPY.removeCanvas }));
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(COPY.removeDescription.replace("{{title}}", target.title)));
    fireEvent.click(within(dialog).getByRole("button", { name: COPY.cancel }));

    expect(mocks.removeCanvas).not.toHaveBeenCalled();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId(REMOVE_ROW_TEST_ID)).toBeTruthy();

    fireEvent.click(
      within(screen.getByTestId(REMOVE_ROW_TEST_ID)).getByRole("button", {
        name: COPY.removeCanvas,
      }),
    );
    fireEvent.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: COPY.removeCanvas,
      }),
    );

    await waitFor(() => expect(mocks.removeCanvas).toHaveBeenCalledWith(target.id));
    await waitFor(() => expect(screen.queryByTestId(REMOVE_ROW_TEST_ID)).toBeNull());
  });

  it("renders a localized load error and keeps the page usable", async () => {
    mocks.listWorkspaceCanvases.mockRejectedValue(new ApiError("server unavailable", 500, {}));

    render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(COPY.loadFailed);
    expect((screen.getByRole("button", { name: COPY.refresh }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("navigates to the task and session returned by edit", async () => {
    const target = canvas({ id: "edit-canvas", title: "Editable canvas" });
    mocks.listWorkspaceCanvases.mockResolvedValue(response([target]));
    mocks.startCanvasEdit.mockResolvedValue({
      canvas_id: target.id,
      task_id: "task/created for edit",
      session_id: "session/created",
    });

    render(<WorkspaceCanvasesPage workspaceId="workspace-1" />);
    const row = await screen.findByTestId("workspace-canvas-edit-canvas");
    fireEvent.click(within(row).getByRole("button", { name: COPY.editCanvas }));

    await waitFor(() => expect(mocks.startCanvasEdit).toHaveBeenCalledWith(target.id));
    expect(mocks.push).toHaveBeenCalledWith(
      "/t/task%2Fcreated%20for%20edit?sessionId=session%2Fcreated",
    );
  });
});
