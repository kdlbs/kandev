import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { PluginConfigField } from "@/lib/plugins/config-schema";
import { PluginConfigForm } from "./plugin-config-form";

const { listAgents, listExecutors, listAvailableAgents } = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listExecutors: vi.fn(),
  listAvailableAgents: vi.fn(),
}));

vi.mock("@/lib/api", () => ({ listAgents, listExecutors, listAvailableAgents }));
vi.mock("@/lib/api/domains/utility-api", () => ({ listUtilityAgents: vi.fn() }));

const AGENT = {
  id: "agent-1",
  name: "claude",
  display_name: "Claude",
  profiles: [
    { id: "profile-1", agentDisplayName: "Claude", name: "Coordinator", enabled: true },
    { id: "profile-2", agentDisplayName: "Claude", name: "Retired", enabled: false },
  ],
};

const agentProfileField: PluginConfigField = {
  name: "agent_profile",
  label: "Agent profile",
  type: "agent_profile",
  required: false,
  secret: false,
};

beforeEach(() => {
  listAgents.mockResolvedValue({ agents: [AGENT] });
  listExecutors.mockResolvedValue({ executors: [] });
  listAvailableAgents.mockResolvedValue({ agents: [] });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderForm(value = "") {
  return render(
    <StateProvider>
      <PluginConfigForm
        fields={[agentProfileField]}
        values={{ agent_profile: value }}
        initialValues={{ agent_profile: value }}
        disabled={false}
        onChange={vi.fn()}
      />
    </StateProvider>,
  );
}

describe("PluginConfigForm agent_profile field", () => {
  // The plugin settings route does not fetch settings data itself, so the
  // field has to load agent profiles or an operator arriving by direct URL
  // gets an empty picker and cannot configure the plugin at all.
  it("loads agent profiles instead of relying on an already-hydrated store", async () => {
    renderForm();
    await waitFor(() => expect(listAgents).toHaveBeenCalled());
  });

  it("renders the saved profile's label once profiles resolve", async () => {
    renderForm("profile-1");
    expect(await screen.findByText(/Coordinator/)).toBeTruthy();
  });
});
