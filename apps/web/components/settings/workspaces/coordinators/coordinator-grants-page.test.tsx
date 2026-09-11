import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AuditEventDTO, GrantDTO } from "@/lib/api/domains/coordinator-api";

const mocks = vi.hoisted(() => ({
  listWorkspaceCoordinatorGrants: vi.fn(),
  listWorkspaceCoordinatorAudit: vi.fn(),
  revokeCoordinatorGrant: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => true,
}));

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listWorkspaceCoordinatorGrants: mocks.listWorkspaceCoordinatorGrants,
  listWorkspaceCoordinatorAudit: mocks.listWorkspaceCoordinatorAudit,
  revokeCoordinatorGrant: mocks.revokeCoordinatorGrant,
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}));

vi.mock("./create-grant-dialog", () => ({
  CreateGrantDialog: () => null,
}));

import { CoordinatorGrantsPage } from "./coordinator-grants-page";

const ACTIVE_GRANT: GrantDTO = {
  id: "grant-1",
  coordinator_task_id: "task-1",
  principal_id: "principal-1",
  workspace_id: "workspace-1",
  scope_kind: "workspace",
  scope_id: "workspace-1",
  capabilities: "inspect,orchestrate",
  note: "",
  granted_by_user_id: "admin",
  granted_at: "2026-09-05T10:00:00Z",
};

const AUDIT_EVENT: AuditEventDTO = {
  id: "audit-1",
  occurred_at: "2026-09-05T10:00:00Z",
  principal_id: "principal-1",
  actor_task_id: "task-1",
  actor_session_id: "session-1",
  target_task_id: "task-2",
  workspace_id: "workspace-1",
  action: "stop_task",
  capability: "orchestrate",
  decision: "allowed",
  grant_id: "grant-1",
  result: "ok",
  detail: "grant",
};

function resetMocks() {
  vi.clearAllMocks();
  mocks.listWorkspaceCoordinatorGrants.mockResolvedValue({ grants: [ACTIVE_GRANT], total: 1 });
  mocks.listWorkspaceCoordinatorAudit.mockResolvedValue({ events: [AUDIT_EVENT], total: 1 });
  mocks.revokeCoordinatorGrant.mockResolvedValue({ id: ACTIVE_GRANT.id, revoked: true });
}

describe("CoordinatorGrantsPage", () => {
  beforeEach(resetMocks);

  afterEach(cleanup);

  it("renders an audit loading error instead of an empty audit log when audit fetch fails", async () => {
    mocks.listWorkspaceCoordinatorAudit.mockRejectedValueOnce(new Error("audit unavailable"));

    render(<CoordinatorGrantsPage workspaceId="workspace-1" />);

    await waitFor(() => expect(mocks.listWorkspaceCoordinatorAudit).toHaveBeenCalledTimes(1));
    const auditSection = screen.getByTestId("coordinator-audit-section");
    expect(within(auditSection).getByText("Error loading")).toBeTruthy();
    expect(within(auditSection).queryByText("No audit events recorded yet.")).toBeNull();
  });

  it("requires confirmation before revoking a coordinator grant", async () => {
    render(<CoordinatorGrantsPage workspaceId="workspace-1" />);

    const row = await screen.findByTestId("grant-row-grant-1");
    fireEvent.click(within(row).getByTestId("revoke-grant-grant-1"));

    expect(mocks.revokeCoordinatorGrant).not.toHaveBeenCalled();

    const confirmation = await screen.findByTestId("revoke-grant-confirm-popover");
    expect(confirmation.textContent).toContain("Revoke grant?");

    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(mocks.revokeCoordinatorGrant).not.toHaveBeenCalled();

    fireEvent.click(within(row).getByTestId("revoke-grant-grant-1"));
    fireEvent.click(
      within(await screen.findByTestId("revoke-grant-confirm-popover")).getByTestId(
        "revoke-grant-confirm",
      ),
    );

    await waitFor(() => expect(mocks.revokeCoordinatorGrant).toHaveBeenCalledWith("grant-1"));
    await waitFor(() => expect(mocks.listWorkspaceCoordinatorGrants).toHaveBeenCalledTimes(2));
  });
});
