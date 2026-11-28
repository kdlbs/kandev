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
const CREDENTIAL_ROW = "automation-workspace-credential-SERVICE_KEY";
const CREDENTIAL_GRANT_BUTTON = "automation-workspace-credential-grant-SERVICE_KEY";
const CREDENTIAL_REVOKE_BUTTON = "automation-workspace-credential-revoke-SERVICE_KEY";
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
        workspace_owner_revision: 7,
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

    fireEvent.click(screen.getByTestId(CREDENTIAL_REVOKE_BUTTON));

    await waitFor(() => {
      expect(mocks.revokeWorkspaceCredential).toHaveBeenCalledWith(AUTOMATION_ID, {
        repository_id: REPOSITORY_ID,
        key: "SERVICE_KEY",
        expected_automation_revision: 3,
        expected_credential_revision: 2,
        expected_workspace_owner_revision: 7,
      });
    });
    expect(onAutomationChange).toHaveBeenCalledWith(updated);
  });

  it("names the creator and returns focus to the row when its grant dialog closes", async () => {
    renderChoices({ state: "needs_approval", can_grant: true, can_revoke: false });
    const row = screen.getByTestId(CREDENTIAL_ROW);

    fireEvent.click(screen.getByTestId(CREDENTIAL_GRANT_BUTTON));

    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain(CREATOR_NAME);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(row));
  });

  it("returns phone drawer focus to the originating row", async () => {
    mocks.isMobile = true;
    renderChoices({ state: "needs_approval", can_grant: true, can_revoke: false });
    const row = screen.getByTestId(CREDENTIAL_ROW);

    fireEvent.click(screen.getByTestId(CREDENTIAL_GRANT_BUTTON));

    const drawer = await screen.findByTestId("automation-workspace-credential-confirmation-drawer");
    expect(drawer.textContent).toContain(CREATOR_NAME);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(drawer.getAttribute("data-state")).toBe("closed");
    fireEvent.animationEnd(drawer);

    await waitFor(() => expect(document.activeElement).toBe(row));
  });

  it("does not expose workspace grant or revoke actions to a non-owner", () => {
    render(
      <WorkspaceCredentialChoices
        automation={automation({ state: "stale", can_grant: true, can_revoke: true })}
        repositories={[repository]}
        selectedRepositoryIds={[REPOSITORY_ID]}
        isWorkspaceOwner={false}
        onAutomationChange={vi.fn()}
      />,
    );

    expect(screen.queryByTestId(CREDENTIAL_GRANT_BUTTON)).toBeNull();
    expect(screen.queryByTestId(CREDENTIAL_REVOKE_BUTTON)).toBeNull();
  });

  it("submits the binding and owner revisions shown when grant confirmation opened", async () => {
    mocks.grantWorkspaceCredential.mockResolvedValue(automation({}));
    renderChoices({ state: "needs_approval", can_grant: true, can_revoke: false });

    fireEvent.click(screen.getByTestId(CREDENTIAL_GRANT_BUTTON));
    fireEvent.click(await screen.findByRole("button", { name: "Grant use" }));

    await waitFor(() => {
      expect(mocks.grantWorkspaceCredential).toHaveBeenCalledWith(AUTOMATION_ID, {
        repository_id: REPOSITORY_ID,
        key: "SERVICE_KEY",
        expected_automation_revision: 3,
        expected_credential_revision: 2,
        expected_binding_updated_at: "2026-11-27T10:00:00Z",
        expected_workspace_owner_revision: 7,
      });
    });
  });
});
