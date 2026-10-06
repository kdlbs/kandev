import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SnapshotInfo } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  useBackups: vi.fn(),
  createBackup: vi.fn(),
  deleteBackup: vi.fn(),
}));

// Mirrors the auth slice the admin gate reads. `undefined` is the
// auth-disabled single-user mode, which the backend treats as an admin.
let currentRole: "admin" | "member" | undefined;
let currentMode: "disabled" | "setup" | "enabled" = "enabled";

// Mirrors the auth slice the admin gate reads. `currentMode` distinguishes
// auth-disabled single-user mode (synthetic admin) from a cleared session.
vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { auth: { mode: string; user?: { role: string } } }) => unknown,
  ) =>
    selector({
      auth: { mode: currentMode, user: currentRole ? { role: currentRole } : undefined },
    }),
}));

vi.mock("@/hooks/domains/system/use-backups", () => ({
  useBackups: mocks.useBackups,
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  buildBackupDownloadUrl: vi.fn((name: string) => `/backups/${name}`),
  createBackup: mocks.createBackup,
  deleteBackup: mocks.deleteBackup,
}));

vi.mock("./job-progress-indicator", () => ({
  JobProgressIndicator: () => null,
}));

vi.mock("./restore-dialog", () => ({
  RestoreDialog: () => null,
}));

import { BackupsTable } from "./backups-table";

const SNAPSHOT: SnapshotInfo = {
  name: "manual-20260101-000000.db",
  kind: "manual",
  size_bytes: 2048,
  mtime: "2026-01-01T00:00:00Z",
};

const CREATE_TEST_ID = "system-backups-create";
const DOWNLOAD_TEST_ID = "system-backups-download";
const RESTORE_TEST_ID = "system-backups-restore";
const DELETE_TEST_ID = "system-backups-delete";
const NAME_TEST_ID = "system-backups-name";

function headerCellCount(): number {
  return screen.getAllByRole("columnheader").length;
}

function bodyCellCount(): number {
  return screen.getByTestId("system-backups-row").querySelectorAll("td").length;
}

function renderBackupsTable() {
  return render(
    <TooltipProvider delayDuration={0}>
      <BackupsTable />
    </TooltipProvider>,
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(cleanup);

beforeEach(() => {
  mocks.createBackup.mockClear();
  mocks.deleteBackup.mockClear();
  mocks.useBackups.mockReturnValue({
    backups: [SNAPSHOT],
    loaded: true,
    isLoading: false,
    reload: vi.fn(),
    reloadAfterWrite: vi.fn().mockResolvedValue([]),
    captureScope: vi.fn(() => ({ identityKey: "current" })),
    isCurrentScope: vi.fn(() => true),
    scopeIdentityKey: "current",
    scopeGeneration: 0,
  });
  mocks.createBackup.mockResolvedValue({ job_id: "backup-job" });
  mocks.deleteBackup.mockResolvedValue(undefined);
});

describe("BackupsTable permissions", () => {
  // Create, download, restore, and delete are admin-only on the backend
  // because a snapshot is a copy of the whole multi-user database. A member
  // must not be offered a control that can only answer 403.
  it("hides every mutating control from a member but keeps the listing", () => {
    currentRole = "member";
    currentMode = "enabled";

    renderBackupsTable();

    expect(screen.queryByTestId(CREATE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(DOWNLOAD_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RESTORE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(DELETE_TEST_ID)).toBeNull();
    expect(screen.getByTestId(NAME_TEST_ID).textContent).toBe(SNAPSHOT.name);
    expect(screen.getByTestId("system-backups-admin-only")).toBeTruthy();
    // Header and body must agree: an empty action cell with no header leaves
    // an unlabeled column for assistive technology.
    expect(headerCellCount()).toBe(bodyCellCount());
  });

  it("offers every control to an admin", () => {
    currentRole = "admin";
    currentMode = "enabled";

    renderBackupsTable();

    expect(screen.getByTestId(CREATE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DOWNLOAD_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(RESTORE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DELETE_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("system-backups-admin-only")).toBeNull();
    expect(headerCellCount()).toBe(bodyCellCount());
  });

  // Auth disabled: no user in the boot payload, and the backend's synthetic
  // identity is an admin. Nothing may change.
  it("offers every control when no user is signed in", () => {
    currentRole = undefined;
    currentMode = "disabled";

    renderBackupsTable();

    expect(screen.getByTestId(CREATE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DOWNLOAD_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(RESTORE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DELETE_TEST_ID)).toBeTruthy();
  });
});

describe("BackupsTable mutation results", () => {
  it("starts create polling with a read boundary after POST acceptance", async () => {
    currentRole = "admin";
    currentMode = "enabled";
    const oldSnapshot = { ...SNAPSHOT, name: "manual-old.db" };
    const reload = vi.fn().mockResolvedValue([SNAPSHOT]);
    const reloadAfterWrite = vi.fn().mockResolvedValue([SNAPSHOT]);
    mocks.useBackups.mockReturnValue({
      backups: [oldSnapshot],
      loaded: true,
      isLoading: false,
      reload,
      reloadAfterWrite,
      captureScope: vi.fn(() => ({ identityKey: "current" })),
      isCurrentScope: vi.fn(() => true),
    });

    renderBackupsTable();
    fireEvent.click(screen.getByTestId(CREATE_TEST_ID));

    await waitFor(() => expect(reloadAfterWrite).toHaveBeenCalledOnce());
    expect(reload).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.getByTestId(CREATE_TEST_ID).textContent).not.toContain("Creating"),
    );
  });

  it("refreshes after successful delete and does not refresh after failed delete", async () => {
    currentRole = "admin";
    currentMode = "enabled";
    const reload = vi.fn().mockResolvedValue([SNAPSHOT]);
    const reloadAfterWrite = vi.fn().mockResolvedValue([SNAPSHOT]);
    mocks.useBackups.mockReturnValue({
      backups: [SNAPSHOT],
      loaded: true,
      isLoading: false,
      reload,
      reloadAfterWrite,
      captureScope: vi.fn(() => ({ identityKey: "current" })),
      isCurrentScope: vi.fn(() => true),
    });
    renderBackupsTable();

    mocks.deleteBackup.mockResolvedValueOnce(undefined);
    fireEvent.click(screen.getByTestId(DELETE_TEST_ID));
    await waitFor(() => expect(reloadAfterWrite).toHaveBeenCalledOnce());
    expect(reload).not.toHaveBeenCalled();

    mocks.deleteBackup.mockRejectedValueOnce(new Error("delete failed"));
    fireEvent.click(screen.getByTestId(DELETE_TEST_ID));
    await waitFor(() =>
      expect(screen.getByTestId("system-backups-error").textContent).toBe("delete failed"),
    );
    expect(reloadAfterWrite).toHaveBeenCalledOnce();
  });
});

describe("BackupsTable identity fencing", () => {
  it.each(["create", "delete"] as const)(
    "does not refresh or surface an obsolete %s result after identity changes",
    async (operation) => {
      currentRole = "admin";
      currentMode = "enabled";
      let identity = "user-1";
      const reloadAfterWrite = vi.fn().mockResolvedValue([]);
      const useBackupsValue = () => ({
        backups: [SNAPSHOT],
        loaded: true,
        isLoading: false,
        reload: vi.fn().mockResolvedValue([SNAPSHOT]),
        reloadAfterWrite,
        captureScope: () => ({ identityKey: identity }),
        isCurrentScope: (scope: { identityKey: string }) => scope.identityKey === identity,
        scopeIdentityKey: identity,
      });
      mocks.useBackups.mockImplementation(useBackupsValue);
      const pending = deferred<{ job_id: string } | undefined>();
      if (operation === "create") mocks.createBackup.mockReturnValueOnce(pending.promise);
      else mocks.deleteBackup.mockReturnValueOnce(pending.promise);

      const view = renderBackupsTable();
      fireEvent.click(screen.getByTestId(operation === "create" ? CREATE_TEST_ID : DELETE_TEST_ID));
      await waitFor(() =>
        expect(
          operation === "create" ? mocks.createBackup : mocks.deleteBackup,
        ).toHaveBeenCalledOnce(),
      );

      identity = "user-2";
      view.rerender(
        <TooltipProvider delayDuration={0}>
          <BackupsTable />
        </TooltipProvider>,
      );
      await act(async () =>
        pending.resolve(operation === "create" ? { job_id: "late" } : undefined),
      );

      expect(reloadAfterWrite).not.toHaveBeenCalled();
      expect(screen.queryByTestId("system-backups-error")).toBeNull();
      if (operation === "create") {
        expect(screen.getByTestId(CREATE_TEST_ID).textContent).not.toContain("Creating");
      }
    },
  );
});

describe("BackupsTable row actions", () => {
  it("describes each row action and keeps its accessible name on the touch target", async () => {
    currentRole = "admin";
    currentMode = "enabled";

    renderBackupsTable();

    const actions = [
      [DOWNLOAD_TEST_ID, "Download", `Download ${SNAPSHOT.name}`],
      [RESTORE_TEST_ID, "Restore", `Restore ${SNAPSHOT.name}`],
      [DELETE_TEST_ID, "Delete", `Delete ${SNAPSHOT.name}`],
    ] as const;
    for (const [testId, operation, label] of actions) {
      const action = screen.getByTestId(testId);
      expect(action.getAttribute("aria-label")).toBe(label);
      expect(action.className).toContain("[@media(pointer:coarse)]:h-11");
      expect(action.className).toContain("[@media(pointer:coarse)]:w-11");

      fireEvent.focus(action);
      const tooltip = await screen.findByRole("tooltip");
      expect(tooltip.textContent).toBe(operation);
      expect(tooltip.textContent).not.toContain(SNAPSHOT.name);
      fireEvent.blur(action);
    }
  });
});
