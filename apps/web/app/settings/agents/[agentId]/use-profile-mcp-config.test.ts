import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { updateAgentProfileMcpConfigAction } from "@/app/actions/agents";
import type { AgentProfileMcpConfig } from "@/lib/types/http";
import { useProfileMcpConfig } from "./use-profile-mcp-config";

vi.mock("@/app/actions/agents", () => ({
  getAgentProfileMcpConfigAction: vi.fn(),
  updateAgentProfileMcpConfigAction: vi.fn(),
}));

describe("useProfileMcpConfig", () => {
  it("preserves an MCP edit made while save is in flight", async () => {
    let finishSave: (config: AgentProfileMcpConfig) => void = () => undefined;
    vi.mocked(updateAgentProfileMcpConfigAction).mockImplementation(
      () =>
        new Promise((resolve) => {
          finishSave = resolve;
        }),
    );
    const initial = config({ command: "initial" });
    const { result } = renderHook(() =>
      useProfileMcpConfig({
        profileId: "profile-1",
        supportsMcp: true,
        initialConfig: initial,
        onToastError: vi.fn(),
      }),
    );

    act(() => result.current.handleMcpServersChange(serverText("submitted")));
    let savePromise!: Promise<void>;
    act(() => {
      savePromise = result.current.handleSaveMcp();
    });
    act(() => result.current.handleMcpServersChange(serverText("newer")));
    await act(async () => {
      finishSave(config({ command: "submitted" }));
      await savePromise;
    });

    expect(result.current.mcpServers).toContain("newer");
    expect(result.current.mcpDirty).toBe(true);
  });
});

function config(server: Record<string, unknown>): AgentProfileMcpConfig {
  return { profile_id: "profile-1", enabled: true, servers: { test: server } };
}

function serverText(command: string): string {
  return JSON.stringify({ mcpServers: { test: { command } } });
}
