import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Canvas, CanvasRelease } from "@/lib/api/domains/canvas-api";

const {
  mockRequestCanvasPromotion,
  mockConfirmCanvasPromotion,
  mockListCanvasReleases,
  mockApproveCanvasRelease,
  mockRejectCanvasRelease,
  mockRollbackCanvas,
} = vi.hoisted(() => ({
  mockRequestCanvasPromotion: vi.fn(),
  mockConfirmCanvasPromotion: vi.fn(),
  mockListCanvasReleases: vi.fn(),
  mockApproveCanvasRelease: vi.fn(),
  mockRejectCanvasRelease: vi.fn(),
  mockRollbackCanvas: vi.fn(),
}));

const translate = vi.hoisted(() => {
  const translations: Record<string, string> = {
    "canvases:runtimeTokenExpired": "The runtime token expired. Try again.",
    "canvases:actionFailed": "The canvas action failed.",
    "canvases:promoteCanvas": "Promote canvas",
    "canvases:promoteCanvasDescription": "Review permissions before promotion.",
    "canvases:loadingPermissions": "Loading requested permissions",
    "canvases:promotionScopeChange": "Promotion changes the canvas scope.",
    "canvases:noAdditionalPermissions": "No additional permissions were requested.",
    "canvases:permissionDeclaration": "Declared permissions",
    "canvases:missingPermissions": "Permissions still needed",
    "canvases:promotingCanvas": "Promoting canvas",
    "canvases:confirmPromotion": "Confirm promotion",
    "canvases:releasesAndPermissions": "Releases and permissions",
    "canvases:releasesDescription": "Review releases and permissions.",
    "canvases:loadingReleases": "Loading releases",
    "canvases:noReleases": "No releases are available.",
    "canvases:approveRelease": "Approve release",
    "canvases:rejectRelease": "Reject release",
    "canvases:rollbackRelease": "Roll back release",
    "canvases:statusActive": "Active",
    "canvases:statusPending": "Pending",
    "canvases:invalidRelease": "Invalid release",
    "canvases:unavailable": "Unavailable",
    "canvases:permissionReads": "API reads",
    "canvases:permissionWrites": "API writes",
    "canvases:permissionEvents": "Events",
    "canvases:permissionExternalOrigins": "External origins",
    "canvases:sharedState": "Shared state",
    "canvases:promotionSourceScope": "Source scope",
    "canvases:promotionSourceActor": "Source actor",
    "canvases:promotionSourceTask": "Source task",
    "canvases:promotionSourceSession": "Source session",
    "canvases:promotionTargetScope": "Target scope",
    "canvases:promotionPlacement": "Workspace placement",
    "common:cancel": "Cancel",
    "common:close": "Close",
  };
  return (key: string) => translations[key] ?? key;
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: translate }),
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children, ...props }: { children: ReactNode }) => (
    <section {...props}>{children}</section>
  ),
  DialogDescription: ({ children }: { children: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children: ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@/lib/api/domains/canvas-api", () => ({
  approveCanvasRelease: mockApproveCanvasRelease,
  confirmCanvasPromotion: mockConfirmCanvasPromotion,
  listCanvasReleases: mockListCanvasReleases,
  rejectCanvasRelease: mockRejectCanvasRelease,
  requestCanvasPromotion: mockRequestCanvasPromotion,
  rollbackCanvas: mockRollbackCanvas,
}));

import { CanvasPromotionDialog, CanvasReleaseDialog } from "./canvas-lifecycle-dialogs";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "plugin-1",
  workspace_id: "workspace-1",
  task_id: "task-1",
  scope_kind: "task",
  title: "Task canvas",
  status: "active",
  active_release_id: "release-1",
  active_release_status: "valid",
};

const COPY = {
  actionFailed: "The canvas action failed.",
  noReleases: "No releases are available.",
  approveRelease: "Approve release",
  rejectRelease: "Reject release",
  rollbackRelease: "Roll back release",
  promotingCanvas: "Promoting canvas",
  confirmPromotion: "Confirm promotion",
  cancel: "Cancel",
} as const;

const TASKS_READ_PERMISSION = "tasks.read";
const EXTERNAL_ORIGIN = "https://example.test";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function release(overrides: Partial<CanvasRelease> = {}): CanvasRelease {
  return {
    id: "release-pending",
    validation_status: "pending_permission",
    ...overrides,
  };
}

beforeEach(() => {
  mockRequestCanvasPromotion.mockReset().mockResolvedValue({
    canvas_id: "canvas-1",
    title: "Task canvas",
    origin_task_id: "origin-task-1",
    source_actor_kind: "task_agent",
    source_user_id: "user-1",
    source_task_id: "source-task-1",
    source_session_id: "source-session-1",
    current_scope: "task",
    target_scope: "workspace",
    placement: "workspace_sidebar",
    permissions: {
      reads: [TASKS_READ_PERMISSION],
      writes: ["tasks.write"],
      events: ["task.updated"],
      shared_state: true,
      external_origins: [EXTERNAL_ORIGIN],
    },
  });
  mockConfirmCanvasPromotion.mockReset();
  mockListCanvasReleases.mockReset().mockResolvedValue({ releases: [] });
  mockApproveCanvasRelease.mockReset();
  mockRejectCanvasRelease.mockReset();
  mockRollbackCanvas.mockReset();
});

afterEach(() => cleanup());

describe("CanvasPromotionDialog", () => {
  it("shows promotion source scope, task/session, placement, and every permission group", async () => {
    render(<CanvasPromotionDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    await waitFor(() =>
      expect(screen.getByTestId("canvas-promotion-source-scope").textContent).toContain("task"),
    );

    expect(screen.getByTestId("canvas-promotion-source-task").textContent).toContain(
      "source-task-1",
    );
    expect(screen.getByTestId("canvas-promotion-source-session").textContent).toContain(
      "source-session-1",
    );
    expect(screen.getByTestId("canvas-promotion-placement").textContent).toContain(
      "workspace_sidebar",
    );
    expect(screen.getByText("API reads")).toBeTruthy();
    expect(screen.getByText("tasks.read")).toBeTruthy();
    expect(screen.getByText("API writes")).toBeTruthy();
    expect(screen.getByText("tasks.write")).toBeTruthy();
    expect(screen.getByText("Events")).toBeTruthy();
    expect(screen.getByText("task.updated")).toBeTruthy();
    expect(screen.getByText("Shared state")).toBeTruthy();
    expect(screen.getByText("External origins")).toBeTruthy();
    expect(screen.getByText("https://example.test")).toBeTruthy();
  });

  it("maps stable API error codes to localized copy", async () => {
    mockRequestCanvasPromotion.mockRejectedValue(
      new ApiError("raw server failure", 401, { error_code: "runtime_token_expired" }),
    );

    render(<CanvasPromotionDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("The runtime token expired. Try again.");
    expect(alert.textContent).not.toContain("raw server failure");
  });

  it("confirms promotion, reports the completed canvas, and closes", async () => {
    const preview = deferred<{
      canvas_id: string;
      current_scope: string;
      target_scope: string;
      active_release_id: string;
      permission_digest: string;
      grant_generation: number;
      permissions: { reads: string[] };
    }>();
    const confirmation = deferred<Canvas>();
    const promoted = { ...canvas, scope_kind: "workspace" };
    const onCompleted = vi.fn();
    const onOpenChange = vi.fn();
    mockRequestCanvasPromotion.mockReturnValue(preview.promise);
    mockConfirmCanvasPromotion.mockReturnValue(confirmation.promise);

    render(
      <CanvasPromotionDialog
        canvas={canvas}
        open
        onOpenChange={onOpenChange}
        onCompleted={onCompleted}
      />,
    );

    const confirmButton = screen.getByRole("button", { name: COPY.confirmPromotion });
    expect((confirmButton as HTMLButtonElement).disabled).toBe(true);

    preview.resolve({
      canvas_id: canvas.id,
      current_scope: "task",
      target_scope: "workspace",
      active_release_id: "release-1",
      permission_digest: "digest-1",
      grant_generation: 7,
      permissions: { reads: [] },
    });
    await waitFor(() => expect((confirmButton as HTMLButtonElement).disabled).toBe(false));

    fireEvent.click(confirmButton);
    await waitFor(() =>
      expect(mockConfirmCanvasPromotion).toHaveBeenCalledWith(canvas.id, {
        expected_release_id: "release-1",
        expected_permission_digest: "digest-1",
        expected_grant_generation: 7,
      }),
    );
    expect(
      (screen.getByRole("button", { name: COPY.promotingCanvas }) as HTMLButtonElement).disabled,
    ).toBe(true);

    confirmation.resolve(promoted);
    await waitFor(() => expect(onCompleted).toHaveBeenCalledWith(promoted));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("closes promotion without confirming when cancelled", async () => {
    const onOpenChange = vi.fn();
    render(<CanvasPromotionDialog canvas={canvas} open onOpenChange={onOpenChange} />);

    fireEvent.click(await screen.findByRole("button", { name: COPY.cancel }));

    expect(mockConfirmCanvasPromotion).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("CanvasReleaseDialog validation", () => {
  it("localizes stable release validation codes", async () => {
    mockListCanvasReleases.mockResolvedValue({
      releases: [
        {
          id: "release-1",
          validation_status: "unavailable",
          validation_error: "runtime_token_expired",
        },
      ],
    });

    render(<CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    await waitFor(() =>
      expect(screen.getByText("The runtime token expired. Try again.")).toBeTruthy(),
    );
  });
});

describe("CanvasReleaseDialog permission review", () => {
  it("shows declared and missing permissions before approving a pending release", async () => {
    mockListCanvasReleases.mockResolvedValue({
      releases: [
        release({
          permissions: {
            reads: [TASKS_READ_PERMISSION],
            writes: ["messages.write"],
            external_origins: [EXTERNAL_ORIGIN],
          },
          missing_permissions: ["api_read:tasks"],
          source_actor_kind: "task_agent",
          source_task_id: "task-1",
          source_session_id: "session-1",
        }),
      ],
    });

    render(<CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    await waitFor(() =>
      expect(screen.getByTestId("canvas-release-permissions-release-pending")).toBeTruthy(),
    );
    expect(screen.getByText("Declared permissions")).toBeTruthy();
    expect(screen.getByText(TASKS_READ_PERMISSION)).toBeTruthy();
    expect(screen.getByText("messages.write")).toBeTruthy();
    expect(screen.getByText(EXTERNAL_ORIGIN)).toBeTruthy();
    expect(screen.getByText("Permissions still needed")).toBeTruthy();
    expect(screen.getByText("api_read:tasks")).toBeTruthy();
    expect(screen.getByRole("button", { name: COPY.approveRelease })).toBeTruthy();
  });

  it("disables release mutations for an archived canvas while keeping the review visible", async () => {
    mockListCanvasReleases.mockResolvedValue({ releases: [release()] });

    render(
      <CanvasReleaseDialog
        canvas={{ ...canvas, status: "archived" }}
        open
        onOpenChange={vi.fn()}
      />,
    );

    const approveButton = await screen.findByRole("button", { name: COPY.approveRelease });
    const rejectButton = screen.getByRole("button", { name: COPY.rejectRelease });
    expect((approveButton as HTMLButtonElement).disabled).toBe(true);
    expect((rejectButton as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("CanvasReleaseDialog actions", () => {
  it.each([
    {
      action: "approve",
      actionMock: mockApproveCanvasRelease,
      buttonName: COPY.approveRelease,
      release: release(),
    },
    {
      action: "reject",
      actionMock: mockRejectCanvasRelease,
      buttonName: COPY.rejectRelease,
      release: release({ id: "release-to-reject" }),
    },
    {
      action: "rollback",
      actionMock: mockRollbackCanvas,
      buttonName: COPY.rollbackRelease,
      release: release({ id: "release-to-rollback", validation_status: "valid" }),
    },
  ])("runs the $action release action and refreshes the list", async (testCase) => {
    const next = { ...canvas, active_release_id: testCase.release.id };
    const onChanged = vi.fn();
    testCase.actionMock.mockResolvedValue(next);
    mockListCanvasReleases
      .mockResolvedValueOnce({ releases: [testCase.release] })
      .mockResolvedValueOnce({ releases: [] });

    render(
      <CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} onChanged={onChanged} />,
    );

    fireEvent.click(await screen.findByRole("button", { name: testCase.buttonName }));

    await waitFor(() =>
      expect(testCase.actionMock).toHaveBeenCalledWith(canvas.id, testCase.release.id),
    );
    await waitFor(() => expect(onChanged).toHaveBeenCalledWith(next));
    await waitFor(() => expect(mockListCanvasReleases).toHaveBeenCalledTimes(2));
    expect(screen.getByText(COPY.noReleases)).toBeTruthy();
  });

  it("disables both actions for a busy pending release until refresh completes", async () => {
    const mutation = deferred<Canvas>();
    const refreshed = deferred<{ releases: CanvasRelease[] }>();
    const pendingRelease = release();
    mockApproveCanvasRelease.mockReturnValue(mutation.promise);
    mockListCanvasReleases
      .mockResolvedValueOnce({ releases: [pendingRelease] })
      .mockReturnValueOnce(refreshed.promise);

    render(<CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    const approveButton = await screen.findByRole("button", { name: COPY.approveRelease });
    const rejectButton = screen.getByRole("button", { name: COPY.rejectRelease });
    fireEvent.click(approveButton);

    await waitFor(() => expect((approveButton as HTMLButtonElement).disabled).toBe(true));
    expect((rejectButton as HTMLButtonElement).disabled).toBe(true);

    mutation.resolve({ ...canvas, active_release_id: pendingRelease.id });
    await waitFor(() => expect(mockListCanvasReleases).toHaveBeenCalledTimes(2));
    expect((approveButton as HTMLButtonElement).disabled).toBe(true);

    refreshed.resolve({ releases: [] });
    await waitFor(() => expect(screen.getByText(COPY.noReleases)).toBeTruthy());
  });

  it("shows a refresh error and releases the busy action after failure", async () => {
    const pendingRelease = release();
    mockListCanvasReleases.mockResolvedValueOnce({ releases: [pendingRelease] });
    mockApproveCanvasRelease.mockRejectedValue(new ApiError("raw action failure", 500, {}));

    render(<CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    const approveButton = await screen.findByRole("button", { name: COPY.approveRelease });
    fireEvent.click(approveButton);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(COPY.actionFailed);
    await waitFor(() => expect((approveButton as HTMLButtonElement).disabled).toBe(false));
    expect(mockListCanvasReleases).toHaveBeenCalledTimes(1);
  });

  it("renders an error when the release list cannot be loaded", async () => {
    mockListCanvasReleases.mockRejectedValue(new ApiError("raw load failure", 500, {}));

    render(<CanvasReleaseDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(COPY.actionFailed);
    expect(screen.queryByRole("status")).toBeNull();
  });
});
