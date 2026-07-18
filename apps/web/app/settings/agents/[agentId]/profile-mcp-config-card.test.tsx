import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { ProfileMcpConfigCard } from "./profile-mcp-config-card";

const DIRTY_ATTRIBUTE = "data-settings-dirty";

afterEach(cleanup);

describe("ProfileMcpConfigCard dirty fields", () => {
  it("marks only the changed MCP control and its owning card dirty", () => {
    render(
      <SettingsSaveProvider>
        <TooltipProvider>
          <ProfileMcpConfigCard
            profileId="profile-1"
            supportsMcp
            initialConfig={{ profile_id: "profile-1", enabled: false, servers: {} }}
            onToastError={vi.fn()}
          />
        </TooltipProvider>
      </SettingsSaveProvider>,
    );

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
