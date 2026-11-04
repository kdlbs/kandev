import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProfileMCPSelectionCard } from "@/app/settings/agents/[agentId]/profile-mcp-selection-card";
import { RepositoryMCPSelectionSection } from "./repository-mcp-selection";
import type { MCPSelectionResponse, MCPServerDefinition } from "@/lib/types/http-mcp";

const mocks = vi.hoisted(() => ({
  selection: null as MCPSelectionResponse | null,
  save: vi.fn(),
  contributor: {} as { save: () => Promise<void>; isDirty: boolean },
}));
vi.mock("@/components/settings/settings-save-provider", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/components/settings/settings-save-provider")>()),
  useSettingsSaveCoordinator: () => ({ status: "idle", exclusiveBusy: false }),
  useSettingsSaveContributor: (value: typeof mocks.contributor) => {
    mocks.contributor = value;
  },
}));
vi.mock("@/hooks/domains/workspace/use-mcp-selection-editor", () => ({
  useMCPSelectionEditor: () => ({
    selection: mocks.selection,
    save: mocks.save,
    loading: false,
    saving: false,
    reload: vi.fn(),
  }),
}));
vi.mock("@/hooks/domains/workspace/use-mcp-workspace-settings", () => ({
  useMCPWorkspaceDefinitions: () => ({
    loading: false,
    definitions: ["a", "b"].map(
      (id) =>
        ({
          id,
          workspace_id: "w",
          display_name: id,
          runtime_name: id,
          enabled: true,
          configuration: {},
          execution_mode: "remote",
          transport: "http",
        }) as MCPServerDefinition,
    ),
  }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "w", items: [{ id: "w", name: "Workspace" }] },
      agentProfiles: { items: [] },
      repositories: { itemsByWorkspaceId: {} },
      kanban: { tasks: [] },
      workflows: { items: [] },
      taskSessions: { items: {} },
    }),
}));

afterEach(cleanup);

beforeEach(() => {
  mocks.selection = { workspace_id: "w", scope: "repository", owner_id: "r", definition_ids: [] };
  mocks.save.mockReset();
});

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1, AC-AGENTS-MCP-ASSIGNMENT-003.4
it.each(["repository", "profile"] as const)(
  "retains a newer %s draft when an earlier save is acknowledged",
  async (scope) => {
    mocks.selection!.scope = scope;
    const content =
      scope === "repository" ? (
        <RepositoryMCPSelectionSection repositoryId="r" workspaceId="w" />
      ) : (
        <ProfileMCPSelectionCard profileId="r" workspaceId="w" />
      );
    let resolve!: (value: MCPSelectionResponse) => void;
    mocks.save.mockReturnValue(
      new Promise<MCPSelectionResponse>((yes) => {
        resolve = yes;
      }),
    );
    const { rerender } = render(content);
    if (scope === "repository") {
      fireEvent.click(screen.getByTestId("repository-mcp-selection-trigger"));
    }
    const boxes = await screen.findAllByRole("checkbox");
    fireEvent.click(boxes[0]);
    let pending!: Promise<void>;
    act(() => {
      pending = mocks.contributor.save();
    });
    fireEvent.click(boxes[1]);
    await act(async () => {
      mocks.selection = { ...mocks.selection!, definition_ids: ["a"] };
      resolve(mocks.selection);
      await pending;
    });
    rerender(content);
    await waitFor(() => expect(mocks.contributor.isDirty).toBe(true));
    expect(screen.getAllByRole("checkbox")[1].getAttribute("data-state")).toBe("checked");
  },
);
