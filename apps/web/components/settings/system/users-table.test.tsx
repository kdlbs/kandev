import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AuthUser } from "@/lib/state/slices/auth/types";

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
  updateUser: vi.fn(),
  toast: vi.fn(),
  responsive: { isFinePointer: true },
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

describe("UsersTable local mutation confirmations", () => {
  beforeEach(() => {
    mocks.responsive.isFinePointer = true;
    vi.clearAllMocks();
    mocks.listUsers.mockResolvedValue({ users: [ADMIN, TARGET] });
    mocks.updateUser.mockResolvedValue({ user: TARGET });
  });

  afterEach(cleanup);

  it("confirms another user's role change at its row action and refreshes the list", async () => {
    await renderUsersTable();
    const row = targetRow();

    fireEvent.click(within(row).getByTestId("users-table-toggle-role"));

    const confirmation = await screen.findByTestId("users-table-confirm-popover");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(confirmation.textContent).toContain(`Change ${TARGET_EMAIL} to admin?`);
    expect(confirmation.textContent).toContain(`${TARGET_EMAIL}; this takes effect immediately.`);

    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.updateUser).toHaveBeenCalledWith("target-id", { role: "admin" });
    });
    await waitFor(() => expect(mocks.listUsers).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId("users-table-confirm-popover")).toBeNull();
  });

  it("morphs a phone row action into visible touch-sized status confirmation", async () => {
    mocks.responsive.isFinePointer = false;
    await renderUsersTable();
    const row = targetRow();

    expect(screen.getByTestId("users-table-mobile-list")).toBeTruthy();
    expect(within(row).getByTestId("users-table-email").textContent).toContain(TARGET.email);
    fireEvent.click(within(row).getByTestId("users-table-toggle-status"));

    const confirmation = await screen.findByTestId("users-table-inline-confirmation");
    expect(confirmation.textContent).toContain(
      `Disable ${TARGET_EMAIL}? They will be signed out everywhere.`,
    );
    expect(within(confirmation).getByTestId(CONFIRM_TEST_ID).className).toContain("h-11");
    expect(within(confirmation).getByTestId(CONFIRM_TEST_ID).className).toContain("min-w-11");
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => {
      expect(mocks.updateUser).toHaveBeenCalledWith("target-id", { status: "disabled" });
    });
  });

  it("keeps existing request error feedback without refreshing after failure", async () => {
    mocks.updateUser.mockRejectedValue(new Error("permission denied"));
    await renderUsersTable();

    const row = targetRow();
    fireEvent.click(within(row).getByTestId("users-table-toggle-status"));
    const confirmation = await screen.findByTestId("users-table-confirm-popover");
    fireEvent.click(within(confirmation).getByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1));
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "error", title: "Could not update user" }),
    );
    expect(mocks.listUsers).toHaveBeenCalledTimes(1);
  });
});
