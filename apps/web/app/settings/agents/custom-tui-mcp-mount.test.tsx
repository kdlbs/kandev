import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Agent } from "@/lib/types/http-agents";

// The strategy list is fetched on mount; the mount assertion below is about
// whether the control is reachable at all, not about the option list.
vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  listMCPStrategies: vi.fn(async () => [{ key: "claude", description: "a config file" }]),
  updateCustomTUIAgentMCPStrategy: vi.fn(),
}));

const { CustomTUIMcpCard } = await import("@/components/settings/custom-tui-mcp-card");
const { InstalledAgentCard } = await import("@/components/settings/installed-agent-card");

const TIMESTAMP = "2026-08-13T10:00:00Z";
const STRATEGY_SELECT = "mcp-strategy-select";

function tuiAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: "agent-1",
    name: "fuel-claude",
    supports_mcp: false,
    tui_config: {
      command: "fuelclaude",
      display_name: "Fuel Claude",
      wait_for_terminal: true,
    },
    profiles: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  } as Agent;
}

function discovery(name: string) {
  return {
    name,
    supports_mcp: false,
    mcp_config_path: null,
    installation_paths: [],
    available: true,
    matched_path: "/usr/bin/fuelclaude",
  };
}

/**
 * Composes the card exactly as the agents index page does: inside
 * InstalledAgentCard, keyed off the saved agent record.
 *
 * The point of asserting through this composition rather than rendering
 * CustomTUIMcpCard alone is that the control was previously mounted on the
 * agent detail route, which redirects saved agents back to the index before the
 * form mounts. Testing the leaf in isolation passed while the control was
 * reachable from nowhere.
 */
function renderIndexComposition(saved: Agent | undefined, name = "fuel-claude") {
  return render(
    <StateProvider>
      <ToastProvider>
        <InstalledAgentCard
          agent={discovery(name) as never}
          savedAgent={saved}
          displayName="Fuel Claude"
        >
          <CustomTUIMcpCard agent={saved} />
        </InstalledAgentCard>
      </ToastProvider>
    </StateProvider>,
  );
}

describe("custom TUI MCP control on the agents index", () => {
  afterEach(cleanup);

  it("renders for a saved custom TUI agent", async () => {
    renderIndexComposition(tuiAgent());
    expect(await screen.findByTestId("agent-mcp-strategy-fuel-claude")).toBeTruthy();
    expect(screen.getByTestId(STRATEGY_SELECT)).toBeTruthy();
  });

  it("shows the agent's stored strategy as the current value", async () => {
    renderIndexComposition(
      tuiAgent({
        supports_mcp: true,
        tui_config: {
          command: "fuelclaude",
          display_name: "Fuel Claude",
          wait_for_terminal: true,
          mcp_strategy: "claude",
        },
      }),
    );
    const trigger = await screen.findByTestId(STRATEGY_SELECT);
    expect(trigger.textContent).toContain("claude");
  });

  it("renders nothing for a built-in (non-TUI) agent", () => {
    renderIndexComposition(tuiAgent({ tui_config: null }), "claude-acp");
    expect(screen.queryByTestId(STRATEGY_SELECT)).toBeNull();
  });

  it("renders nothing for an agent that has not been saved yet", () => {
    renderIndexComposition(undefined);
    expect(screen.queryByTestId(STRATEGY_SELECT)).toBeNull();
  });
});
