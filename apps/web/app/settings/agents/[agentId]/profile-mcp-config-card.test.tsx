import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { getMCPSelections, listMCPServers } from "@/lib/api/domains/mcp-api";
import { softNavigate } from "@/lib/routing/client-router";
import { workspaceId } from "@/lib/types/ids";

import { ProfileMcpConfigCard } from "./profile-mcp-config-card";

const DIRTY_ATTRIBUTE = "data-settings-dirty";

vi.mock("@/lib/api/domains/mcp-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/mcp-api")>()),
  getMCPSelections: vi.fn(),
  listMCPServers: vi.fn(),
}));

afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
  vi.resetAllMocks();
});

describe("ProfileMcpConfigCard dirty fields", () => {
  it("marks only the changed MCP control and its owning card dirty", async () => {
    vi.mocked(listMCPServers).mockResolvedValue([]);
    vi.mocked(getMCPSelections).mockResolvedValue({
      scope: "profile",
      owner_id: "profile-1",
      workspace_id: "workspace-1",
      definition_ids: [],
      legacy_import_status: "pending",
    });
    render(
      <StateProvider
        initialState={{
          workspaces: {
            activeId: null,
            items: [
              {
                id: workspaceId("workspace-1"),
                name: "Workspace",
                owner_id: "owner-1",
                acp_idle_suspension_enabled: false,
                acp_idle_timeout_minutes: 0,
                created_at: "2026-10-26T00:00:00Z",
                updated_at: "2026-10-26T00:00:00Z",
              },
            ],
          },
        }}
      >
        <SettingsSaveProvider>
          <TooltipProvider>
            <ProfileMcpConfigCard
              profileId="profile-1"
              supportsMcp
              initialConfig={{ profile_id: "profile-1", enabled: false, servers: {} }}
              onToastError={vi.fn()}
            />
          </TooltipProvider>
        </SettingsSaveProvider>
      </StateProvider>,
    );

    expect(
      screen.queryByRole("button", { name: "Global profile legacy configuration" }),
    ).toBeNull();
    act(() => softNavigate("/settings/agents/agent-1/profiles/profile-1?workspaceId=workspace-1"));
    fireEvent.click(
      await screen.findByRole("button", { name: "Global profile legacy configuration" }),
    );
    expect(getMCPSelections).toHaveBeenCalledWith("profile", "profile-1", "workspace-1", {
      cache: "no-store",
    });

    const enabled = screen.getByTestId("mcp-enabled");
    const servers = screen.getByTestId("mcp-servers-profile-1");
    const card = enabled.closest('[data-settings-dirty-level="card"]');

    fireEvent.click(enabled);
    expect(enabled.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(servers.getAttribute(DIRTY_ATTRIBUTE)).toBe("false");
    expect(card?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");

    fireEvent.click(enabled);
    expect(card?.getAttribute(DIRTY_ATTRIBUTE)).toBe("false");

    fireEvent.change(servers, {
      target: { value: '{"mcpServers":{"github":{"command":"npx"}}}' },
    });
    expect(enabled.getAttribute(DIRTY_ATTRIBUTE)).toBe("false");
    expect(servers.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(card?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });
});
