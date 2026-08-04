import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AvailableAgent } from "@/lib/types/http-agents";

// Minimal store shape consumed by useAvailableAgents.
type MockStore = {
  availableAgents: {
    items: AvailableAgent[];
    loading: boolean;
    loaded: boolean;
    tools: [];
  };
};

let mockStore: MockStore = {
  availableAgents: { items: [], loading: false, loaded: true, tools: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockStore) => unknown) => selector(mockStore),
}));

import { useAgentProfileOptions } from "./task-create-dialog-options";

function setAvailableAgents(items: AvailableAgent[]) {
  mockStore = { availableAgents: { items, loading: false, loaded: true, tools: [] } };
}

const AGENT_WITH_GPT: AvailableAgent = {
  name: "omp-acp",
  available: true,
  model_config: {
    default_model: "gpt-5",
    available_models: [{ id: "gpt-5", name: "GPT-5" }],
    current_model_id: "gpt-5",
    available_modes: [],
    supports_dynamic_models: false,
    status: "ok",
  },
} as unknown as AvailableAgent;

const GONE_MODEL = "claude-gone";
const DATA_DISABLED = "data-disabled";

function profileOption(overrides: Partial<AgentProfileOption>): AgentProfileOption {
  return {
    id: "profile-1",
    label: "OMP • hybrid",
    agent_id: "agent-1",
    agent_name: "omp-acp",
    cli_passthrough: false,
    ...overrides,
  };
}

function OptionsProbe({ profiles }: { profiles: AgentProfileOption[] }) {
  const options = useAgentProfileOptions(profiles);
  return (
    <div>
      {options.map((option, index) => (
        <div
          key={option.value}
          data-testid={`option-${index}`}
          data-disabled={option.disabled ? "true" : undefined}
          data-reason={option.disabledReason}
        >
          {option.renderLabel()}
        </div>
      ))}
    </div>
  );
}

function renderOptions(profiles: AgentProfileOption[]) {
  render(<OptionsProbe profiles={profiles} />);
  return screen.getByTestId("option-0");
}

beforeEach(() => {
  vi.clearAllMocks();
  setAvailableAgents([AGENT_WITH_GPT]);
});

afterEach(cleanup);

describe("useAgentProfileOptions no-silent-model-fallback gating", () => {
  it("blocks a profile whose start model is gone (strict mode)", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);
    expect(option.getAttribute(DATA_DISABLED)).toBe("true");
    expect(option.getAttribute("data-reason")).toContain(GONE_MODEL);
  });

  it("keeps a profile with an available start model selectable", () => {
    const option = renderOptions([profileOption({ model: "gpt-5" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });

  it("keeps a profile with an empty (agent default) model selectable", () => {
    const option = renderOptions([profileOption({ model: "" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });

  it("keeps a gone-model profile with a fallback selectable and warns", () => {
    const option = renderOptions([
      profileOption({ model: "claude-gone", fallback_model: "gpt-5" }),
    ]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(option.getAttribute("data-reason")).toBeNull();

    // The amber warning icon carries the fallback note.
    expect(screen.getByTitle(/gpt-5/)).not.toBeNull();
  });

  it("keeps a gone-model profile with auto-fallback selectable without a warning", () => {
    const option = renderOptions([profileOption({ model: "claude-gone", auto_fallback: true })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(screen.queryByTitle(/claude-gone/)).toBeNull();
  });

  it("does not gate when the agent model list is unknown (probe not landed)", () => {
    setAvailableAgents([]);
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });
});
