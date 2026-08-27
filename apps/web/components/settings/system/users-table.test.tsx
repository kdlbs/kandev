import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AuthUser } from "@/lib/state/slices/auth/types";

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
  updateUser: vi.fn(),
  toast: vi.fn(),
  responsive: { isFinePointer: true, isMobile: false },
}));

vi.mock("@/lib/api/domains/auth-api", () => ({
  listUsers: mocks.listUsers,
  updateUser: mocks.updateUser,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));

vi.mock("./create-user-dialog", () => ({
  CreateUserDialog: () => null,
}));

vi.mock("./invite-dialog", () => ({
  InviteDialog: () => null,
}));

import { UsersTable } from "./users-table";

const ADMIN = makeUser({ id: "admin-id", email: "admin@example.com", role: "admin" });
const TARGET_EMAIL = "target@example.com";
const TARGET = makeUser({ id: "target-id", email: TARGET_EMAIL, role: "member" });
const CONFIRM_TEST_ID = "users-table-confirm";
const POPOVER_TEST_ID = "users-table-confirm-popover";
const ROLE_TOGGLE_TEST_ID = "users-table-toggle-role";
const STATUS_TOGGLE_TEST_ID = "users-table-toggle-status";

function makeUser(overrides: Partial<AuthUser>): AuthUser {
  return {
    id: "user-id",
    email: "user@example.com",
    display_name: "User",
    role: "member",
    status: "active",
    ...overrides,
  };
}

function targetRow() {
  const row = screen
    .getAllByTestId("users-table-row")
    .find((candidate) => candidate.getAttribute("data-user-id") === TARGET.id);
  if (!row) throw new Error("target user row not found");
  return row;
}

async function renderUsersTable() {
  render(<UsersTable />);
  await screen.findAllByTestId("users-table-row");
  await waitFor(() => expect(mocks.listUsers).toHaveBeenCalledTimes(1));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

function resetMocks() {
  mocks.responsive.isFinePointer = true;
  mocks.responsive.isMobile = false;
  vi.clearAllMocks();
  mocks.listUsers.mockResolvedValue({ users: [ADMIN, TARGET] });
  mocks.updateUser.mockResolvedValue({ user: TARGET });
}

describe("UsersTable confirmation presentation", () => {
  beforeEach(resetMocks);

  afterEach(cleanup);

  it("confirms another user's role change at its row action and refreshes the list", async () => {
    await renderUsersTable();
    const row = targetRow();

    fireEvent.click(within(row).getByTestId(ROLE_TOGGLE_TEST_ID));

    const confirmation = await screen.findByTestId(POPOVER_TEST_ID);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(confirmation.textContent).toContain(`Change ${TARGET_EMAIL} to admin?`);
    expect(confirmation.textContent).toContain(`${TARGET_EMAIL}; this takes effect immediately.`);

    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.updateUser).toHaveBeenCalledWith("target-id", { role: "admin" });
    });
    await waitFor(() => expect(mocks.listUsers).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull();
  });

  it("morphs a phone row action into visible touch-sized status confirmation", async () => {
    mocks.responsive.isFinePointer = false;
    mocks.responsive.isMobile = true;
    await renderUsersTable();
    const row = targetRow();

    expect(screen.getByTestId("users-table-mobile-list")).toBeTruthy();
    expect(within(row).getByTestId("users-table-email").textContent).toContain(TARGET.email);
    fireEvent.click(within(row).getByTestId(STATUS_TOGGLE_TEST_ID));

    const confirmation = await screen.findByTestId("users-table-inline-confirmation");
    expect(confirmation.textContent).toContain(
      `Disable ${TARGET_EMAIL}? They will be signed out everywhere.`,
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.updateUser).toHaveBeenCalledWith("target-id", { status: "disabled" });
    });
  });

  it("chooses row composition by width and confirmation behavior by pointer precision", async () => {
    mocks.responsive.isFinePointer = false;
    await renderUsersTable();

    expect(screen.getByTestId("users-table")).toBeTruthy();
    expect(screen.queryByTestId("users-table-mobile-list")).toBeNull();

    fireEvent.click(within(targetRow()).getByTestId(ROLE_TOGGLE_TEST_ID));
    expect(await screen.findByTestId("users-table-inline-confirmation")).toBeTruthy();
  });

  it("keeps phone cards on a narrow fine-pointer viewport", async () => {
    mocks.responsive.isMobile = true;
    await renderUsersTable();

    expect(screen.getByTestId("users-table-mobile-list")).toBeTruthy();
    expect(screen.queryByTestId("users-table")).toBeNull();

    fireEvent.click(within(targetRow()).getByTestId(ROLE_TOGGLE_TEST_ID));
    expect(await screen.findByTestId(POPOVER_TEST_ID)).toBeTruthy();
  });
});

describe("UsersTable mutation lifecycle", () => {
  beforeEach(resetMocks);

  afterEach(cleanup);

  it("blocks every row mutation until the active update and refresh finish", async () => {
    const update = deferred<{ user: AuthUser }>();
    mocks.updateUser.mockReturnValueOnce(update.promise);
    await renderUsersTable();

    fireEvent.click(within(targetRow()).getByTestId(ROLE_TOGGLE_TEST_ID));
    const confirmation = await screen.findByTestId(POPOVER_TEST_ID);
    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(mocks.updateUser).toHaveBeenCalledTimes(1));
    const statusToggle = within(targetRow()).getByTestId(STATUS_TOGGLE_TEST_ID);
    expect((statusToggle as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(statusToggle);
    expect(mocks.updateUser).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull();

    update.resolve({ user: TARGET });
    await waitFor(() => expect(mocks.listUsers).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        (within(targetRow()).getByTestId(STATUS_TOGGLE_TEST_ID) as HTMLButtonElement).disabled,
      ).toBe(false),
    );
  });

  it("cancels the row confirmation without mutating", async () => {
    await renderUsersTable();

    fireEvent.click(within(targetRow()).getByTestId(ROLE_TOGGLE_TEST_ID));
    const confirmation = await screen.findByTestId(POPOVER_TEST_ID);
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull());
    expect(mocks.updateUser).not.toHaveBeenCalled();
    expect(
      (within(targetRow()).getByTestId(ROLE_TOGGLE_TEST_ID) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("keeps existing request error feedback without refreshing after failure", async () => {
    mocks.updateUser.mockRejectedValue(new Error("permission denied"));
    await renderUsersTable();

    const row = targetRow();
    fireEvent.click(within(row).getByTestId(STATUS_TOGGLE_TEST_ID));
    const confirmation = await screen.findByTestId(POPOVER_TEST_ID);
    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1));
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "error", title: "Could not update user" }),
    );
    expect(mocks.listUsers).toHaveBeenCalledTimes(1);
  });
});
