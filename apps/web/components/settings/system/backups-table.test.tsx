import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SnapshotInfo } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  useBackups: vi.fn(),
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

describe("BackupsTable", () => {
  afterEach(cleanup);

  beforeEach(() => {
    mocks.useBackups.mockReturnValue({
      backups: [SNAPSHOT],
      loaded: true,
      isLoading: false,
      reload: vi.fn(),
    });
  });

  // Create, download, restore, and delete are admin-only on the backend
  // because a snapshot is a copy of the whole multi-user database. A member
  // must not be offered a control that can only answer 403.
  it("hides every mutating control from a member but keeps the listing", () => {
    currentRole = "member";
    currentMode = "enabled";

    render(<BackupsTable />);

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

    render(<BackupsTable />);

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

    render(<BackupsTable />);

    expect(screen.getByTestId(CREATE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DOWNLOAD_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(RESTORE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(DELETE_TEST_ID)).toBeTruthy();
  });
});
