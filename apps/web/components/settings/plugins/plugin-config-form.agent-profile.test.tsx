import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { PluginConfigField } from "@/lib/plugins/config-schema";
import { PluginConfigForm } from "./plugin-config-form";

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));

const AGENT_PROFILE_FIELD = "agent_profile";
const ELIGIBLE_PROFILE_LABEL = "Claude · Notes";

const field: PluginConfigField = {
  name: AGENT_PROFILE_FIELD,
  label: "Notes agent",
  type: "agent_profile",
  required: false,
  secret: false,
};

const profile = (overrides: Partial<AgentProfileOption>): AgentProfileOption => ({
  id: "profile-1",
  label: ELIGIBLE_PROFILE_LABEL,
  agent_id: "claude",
  agent_name: "claude",
  cli_passthrough: false,
  enabled: true,
  ...overrides,
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderField(profiles: AgentProfileOption[], value = "") {
  const onChange = vi.fn<(name: string, value: string | boolean) => void>();
  render(
    <StateProvider initialState={{ agentProfiles: { items: profiles, version: 0 } }}>
      <PluginConfigForm
        fields={[field]}
        values={{ [AGENT_PROFILE_FIELD]: value }}
        initialValues={{ [AGENT_PROFILE_FIELD]: value }}
        disabled={false}
        onChange={onChange}
      />
    </StateProvider>,
  );
  return onChange;
}

describe("PluginConfigForm agent profile field", () => {
  it("offers only enabled global non-CLI profiles and stores the stable ID", () => {
    const onChange = renderField([
      profile({ id: "eligible", label: ELIGIBLE_PROFILE_LABEL }),
      profile({ id: "disabled", label: "Disabled", enabled: false }),
      profile({ id: "workspace", label: "Workspace", workspace_id: "workspace-1" }),
      profile({ id: "cli", label: "CLI", cli_passthrough: true }),
    ]);

    fireEvent.click(screen.getByRole("combobox"));

    expect(screen.getByRole("option", { name: ELIGIBLE_PROFILE_LABEL })).not.toBeNull();
    expect(screen.queryByRole("option", { name: "Disabled" })).toBeNull();
    expect(screen.queryByRole("option", { name: "Workspace" })).toBeNull();
    expect(screen.queryByRole("option", { name: "CLI" })).toBeNull();

    fireEvent.click(screen.getByRole("option", { name: ELIGIBLE_PROFILE_LABEL }));
    expect(onChange).toHaveBeenCalledWith(AGENT_PROFILE_FIELD, "eligible");
  });

  it("shows a stale saved profile without making it selectable", () => {
    renderField([], "deleted-profile");

    expect(screen.getByText("Unavailable: deleted-profile")).not.toBeNull();
  });
});
