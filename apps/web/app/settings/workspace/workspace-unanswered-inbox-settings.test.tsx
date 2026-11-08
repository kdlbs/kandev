import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { WorkspaceState } from "@/lib/state/slices";

const mocks = vi.hoisted(() => ({
  contributor: null as SettingsSaveContributor | null,
  update: vi.fn(),
  setWorkspaces: vi.fn(),
  toast: vi.fn(),
  items: [] as WorkspaceState["items"],
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    mocks.contributor = contributor;
  },
}));

vi.mock("@/components/settings/settings-group", () => ({
  SettingsGroup: ({ title, children }: { title: React.ReactNode; children: React.ReactNode }) => (
    <section>
      <h3>{title}</h3>
      {children}
    </section>
  ),
}));

vi.mock("@/app/actions/workspaces", () => ({
  updateWorkspaceAction: (...args: unknown[]) => mocks.update(...args),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ workspaces: { items: mocks.items }, setWorkspaces: mocks.setWorkspaces }),
  }),
}));

import { WorkspaceUnansweredInboxSettings } from "./workspace-unanswered-inbox-settings";

type Workspace = WorkspaceState["items"][number];

function workspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: "w1",
    name: "Workspace",
    description: null,
    owner_id: "u1",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    ...overrides,
  } as Workspace;
}

beforeEach(() => {
  mocks.contributor = null;
  mocks.update.mockReset();
  mocks.setWorkspaces.mockReset();
  mocks.toast.mockReset();
  mocks.items = [workspace()];
});

afterEach(() => cleanup());

describe("WorkspaceUnansweredInboxSettings", () => {
  it("starts clean, off, with the default retention disabled", () => {
    render(<WorkspaceUnansweredInboxSettings workspace={workspace()} canManage />);
    expect(mocks.contributor?.isDirty).toBe(false);
    const input = screen.getByTestId("workspace-unanswered-retention-input") as HTMLInputElement;
    expect(input.value).toBe("14");
    expect(input.disabled).toBe(true);
  });

  it("saves only the toggled field and updates the workspace store", async () => {
    mocks.update.mockResolvedValue({
      ...workspace(),
      clarification_unanswered_inbox_enabled: true,
      clarification_unanswered_retention_days: 14,
    });
    render(<WorkspaceUnansweredInboxSettings workspace={workspace()} canManage />);
    fireEvent.click(screen.getByTestId("workspace-unanswered-inbox-switch"));
    expect(mocks.contributor?.isDirty).toBe(true);

    await act(async () => {
      await mocks.contributor?.save(mocks.contributor.revision);
    });
    expect(mocks.update).toHaveBeenCalledWith("w1", {
      clarification_unanswered_inbox_enabled: true,
    });
    const items = mocks.setWorkspaces.mock.calls[0][0] as Workspace[];
    expect(items[0].clarification_unanswered_inbox_enabled).toBe(true);
    expect(items[0].clarification_unanswered_retention_days).toBe(14);
    expect(mocks.contributor?.isDirty).toBe(false);
  });

  it("blocks saving an out-of-range retention", () => {
    render(
      <WorkspaceUnansweredInboxSettings
        workspace={workspace({ clarification_unanswered_inbox_enabled: true })}
        canManage
      />,
    );
    fireEvent.change(screen.getByTestId("workspace-unanswered-retention-input"), {
      target: { value: "120" },
    });
    expect(mocks.contributor?.canSave).toBe(false);
    expect(mocks.contributor?.invalidReason).toBe("Enter a whole number from 1 to 90.");
    expect(screen.getByTestId("workspace-unanswered-retention-error")).not.toBeNull();
  });

  it("disables both controls without manage scope", () => {
    render(<WorkspaceUnansweredInboxSettings workspace={workspace()} canManage={false} />);
    const toggle = screen.getByTestId("workspace-unanswered-inbox-switch") as HTMLButtonElement;
    expect(toggle.disabled).toBe(true);
  });
});
