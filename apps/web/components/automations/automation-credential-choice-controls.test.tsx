import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Automation } from "@/lib/types/automation";
import type { Repository } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  grantWorkspaceCredential: vi.fn(),
  revokeWorkspaceCredential: vi.fn(),
  isMobile: false,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile, isFinePointer: !mocks.isMobile }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  grantWorkspaceCredential: mocks.grantWorkspaceCredential,
  revokeWorkspaceCredential: mocks.revokeWorkspaceCredential,
}));

import { WorkspaceCredentialChoices } from "./automation-credential-choice-controls";

const WORKSPACE_ID = "workspace-credential-controls";
const AUTOMATION_ID = "automation-credential-controls";
const REPOSITORY_ID = "repository-credential-controls";
const CREATOR_NAME = "Casey Collaborator";

function automation(choice: Record<string, unknown>): Automation {
  return {
    id: AUTOMATION_ID,
    workspace_id: WORKSPACE_ID,
    resource_revision: 3,
    credential_choices: [
      {
        kind: "workspace",
        repository_id: REPOSITORY_ID,
        key: "SERVICE_KEY",
        state: "needs_approval",
        revision: 2,
        can_grant: false,
        can_revoke: false,
        creator_name: CREATOR_NAME,
        binding_updated_at: "2026-11-27T10:00:00Z",
        workspace_owner_updated_at: "2026-11-27T10:01:00Z",
        ...choice,
      },
    ],
  } as unknown as Automation;
}

const repository = {
  id: REPOSITORY_ID,
  workspace_id: WORKSPACE_ID,
  name: "billing-service",
} as Repository;

function renderChoices(choice: Record<string, unknown>) {
  const onAutomationChange = vi.fn();
  render(
    <WorkspaceCredentialChoices
      automation={automation(choice)}
      repositories={[repository]}
      selectedRepositoryIds={[REPOSITORY_ID]}
      isWorkspaceOwner
      onAutomationChange={onAutomationChange}
    />,
  );
  return onAutomationChange;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.isMobile = false;
});

describe("WorkspaceCredentialChoices", () => {
  it("lets the owner revoke a stale grant when they cannot grant the changed binding", async () => {
    const updated = automation({ state: "revoked", can_revoke: false });
    mocks.revokeWorkspaceCredential.mockResolvedValue(updated);
    const onAutomationChange = renderChoices({
      state: "stale",
      can_grant: false,
      can_revoke: true,
    });

    fireEvent.click(screen.getByTestId("automation-workspace-credential-revoke-SERVICE_KEY"));

    await waitFor(() => {
      expect(mocks.revokeWorkspaceCredential).toHaveBeenCalledWith(AUTOMATION_ID, {
        repository_id: REPOSITORY_ID,
        key: "SERVICE_KEY",
        expected_automation_revision: 3,
        expected_credential_revision: 2,
      });
    });
    expect(onAutomationChange).toHaveBeenCalledWith(updated);
  });

  it("names the creator and returns focus to the row when its grant dialog closes", async () => {
    renderChoices({ state: "needs_approval", can_grant: true, can_revoke: false });
    const row = screen.getByTestId("automation-workspace-credential-SERVICE_KEY");

    fireEvent.click(screen.getByTestId("automation-workspace-credential-grant-SERVICE_KEY"));

    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain(CREATOR_NAME);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(row));
  });

  it("submits the binding and owner revisions shown when grant confirmation opened", async () => {
    mocks.grantWorkspaceCredential.mockResolvedValue(automation({}));
    renderChoices({ state: "needs_approval", can_grant: true, can_revoke: false });

    fireEvent.click(screen.getByTestId("automation-workspace-credential-grant-SERVICE_KEY"));
    fireEvent.click(await screen.findByRole("button", { name: "Grant use" }));

    await waitFor(() => {
      expect(mocks.grantWorkspaceCredential).toHaveBeenCalledWith(AUTOMATION_ID, {
        repository_id: REPOSITORY_ID,
        key: "SERVICE_KEY",
        expected_automation_revision: 3,
        expected_credential_revision: 2,
        expected_binding_updated_at: "2026-11-27T10:00:00Z",
        expected_workspace_owner_updated_at: "2026-11-27T10:01:00Z",
      });
    });
  });
});
