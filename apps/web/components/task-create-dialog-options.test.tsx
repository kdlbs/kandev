import { cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AvailableAgent } from "@/lib/types/http-agents";
import type { Executor } from "@/lib/types/http";
import { computeExecutorHint, useAgentProfileOptions } from "./task-create-dialog-options";

// Minimal store shape consumed by useAvailableAgents.
type MockStore = {
  features: { dynamicAgentRouting: boolean };
  availableAgents: {
    items: AvailableAgent[];
    loading: boolean;
    loaded: boolean;
    tools: [];
  };
};

let mockStore: MockStore = {
  features: { dynamicAgentRouting: true },
  availableAgents: { items: [], loading: false, loaded: true, tools: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockStore) => unknown) => selector(mockStore),
}));

function setAvailableAgents(items: AvailableAgent[]) {
  mockStore = {
    features: { dynamicAgentRouting: true },
    availableAgents: { items, loading: false, loaded: true, tools: [] },
  };
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

describe("computeExecutorHint", () => {
  const executors = [
    { id: "wt", type: "worktree" } as Executor,
    { id: "loc", type: "local" } as Executor,
    { id: "docker", type: "local_docker" } as Executor,
    { id: "remote-docker", type: "remote_docker" } as Executor,
  ];
  const WORKTREE_SINGLE = "A git worktree will be created from the base branch.";
  const WORKTREE_MULTI =
    "A git worktree will be created for each repository in a parent folder. The agent runs in that parent folder so it can see every worktree side by side.";
  const DOCKER =
    "A Docker container will be created from the selected base branch and checked out on a task branch.";
  const LOCAL = "The agent will run directly on the repository.";

  it("returns the multi-repo worktree hint when more than one repo is selected", () => {
    expect(computeExecutorHint(executors, "wt", 2)).toBe(WORKTREE_MULTI);
  });

  it("returns the single-repo worktree hint when exactly one repo is selected", () => {
    expect(computeExecutorHint(executors, "wt", 1)).toBe(WORKTREE_SINGLE);
  });

  it("explains that Docker profiles create an isolated task branch", () => {
    expect(computeExecutorHint(executors, "docker", 1)).toBe(DOCKER);
    expect(computeExecutorHint(executors, "remote-docker", 1)).toBe(DOCKER);
  });

  it("returns the local hint regardless of repoCount", () => {
    expect(computeExecutorHint(executors, "loc", 1)).toBe(LOCAL);
    expect(computeExecutorHint(executors, "loc", 5)).toBe(LOCAL);
  });

  it("returns null for an unknown executor id", () => {
    expect(computeExecutorHint(executors, "nope", 1)).toBeNull();
  });

  it("returns null for an unrecognised executor type", () => {
    const odd = [{ id: "x", type: "remote" as Executor["type"] } as Executor];
    expect(computeExecutorHint(odd, "x", 1)).toBeNull();
  });
});

describe("useAgentProfileOptions enabled filter", () => {
  it("omits disabled profiles from the selectable options", () => {
    const { result } = renderHook(() =>
      useAgentProfileOptions([
        profileOption({ id: "p-enabled", label: "Agent • p-enabled" }),
        profileOption({ id: "p-disabled", label: "Agent • p-disabled", enabled: false }),
        profileOption({ id: "p-legacy", label: "Agent • p-legacy" }),
      ]),
    );
    const ids = result.current.map((o) => o.value);
    expect(ids).toEqual(["p-enabled", "p-legacy"]);
  });

  it("keeps profiles whose enabled flag is absent (legacy options)", () => {
    const { result } = renderHook(() =>
      useAgentProfileOptions([profileOption({ id: "p-legacy", label: "Agent • p-legacy" })]),
    );
    expect(result.current.map((o) => o.value)).toEqual(["p-legacy"]);
  });
});

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
    const option = renderOptions([profileOption({ model: GONE_MODEL, fallback_model: "gpt-5" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(option.getAttribute("data-reason")).toBeNull();

    // The amber warning icon carries the fallback note.
    expect(screen.getByTitle(/gpt-5/)).not.toBeNull();
  });

  it("blocks a profile whose fallback model is also gone (both-gone)", () => {
    const option = renderOptions([
      profileOption({ model: GONE_MODEL, fallback_model: "other-gone" }),
    ]);
    expect(option.getAttribute(DATA_DISABLED)).toBe("true");
    expect(option.getAttribute("data-reason")).toContain(GONE_MODEL);
  });

  it("keeps a gone-model profile with auto-fallback selectable without a warning", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL, auto_fallback: true })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(screen.queryByTitle(/claude-gone/)).toBeNull();
  });

  it("does not gate when the agent model list is unknown (probe not landed)", () => {
    setAvailableAgents([]);
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });
});
