import { cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { AvailableAgent } from "@/lib/types/http-agents";
import type { Executor } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";
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
  agentProfileRecentUse: {
    loaded: boolean;
    records: Record<string, { profileIds: string[]; revision: number; updatedAt: string }>;
  };
};

let mockStore: MockStore = {
  features: { dynamicAgentRouting: true },
  availableAgents: { items: [], loading: false, loaded: true, tools: [] },
  agentProfileRecentUse: { loaded: false, records: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockStore) => unknown) => selector(mockStore),
  useAppStoreApi: () => ({
    getState: () => ({
      agentProfileRecentUse: mockStore.agentProfileRecentUse,
      setAgentProfileRecentUse: vi.fn(),
    }),
  }),
}));

function setAvailableAgents(items: AvailableAgent[]) {
  mockStore = {
    features: { dynamicAgentRouting: true },
    availableAgents: { items, loading: false, loaded: true, tools: [] },
    agentProfileRecentUse: { loaded: false, records: {} },
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
const MODEL_PROBE_WARNING_TEST_ID = "agent-profile-model-probe-warning";

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

function OptionsProbe({
  profiles,
  context,
}: {
  profiles: AgentProfileOption[];
  context?: "task_create";
}) {
  const options = useAgentProfileOptions(profiles, context);
  return (
    <TooltipProvider>
      <div>
        {options.map((option, index) => (
          <div
            key={option.value}
            data-testid={`option-${index}`}
            data-value={option.value}
            data-disabled={option.disabled ? "true" : undefined}
            data-reason={option.disabledReason}
          >
            {option.renderLabel()}
          </div>
        ))}
      </div>
    </TooltipProvider>
  );
}

function renderOptions(profiles: AgentProfileOption[]) {
  render(<OptionsProbe profiles={profiles} />);
  return screen.getByTestId("option-0");
}

function getModelProbeWarning() {
  return screen.getByTestId(MODEL_PROBE_WARNING_TEST_ID);
}

function getModelProbeWarningLabel() {
  return getModelProbeWarning().getAttribute("aria-label");
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

describe("useAgentProfileOptions recent-use ordering", () => {
  it("ranks remembered eligible profiles and keeps unseen source order", () => {
    mockStore.agentProfileRecentUse = {
      loaded: true,
      records: {
        task_create: {
          profileIds: ["missing", "p-disabled", "p-recent"],
          revision: 1,
          updatedAt: "2026-08-27T12:00:00Z",
        },
      },
    };
    render(
      <OptionsProbe
        context="task_create"
        profiles={[
          profileOption({ id: "p-unseen", label: "Agent • unseen" }),
          profileOption({ id: "p-disabled", label: "Agent • disabled", enabled: false }),
          profileOption({ id: "p-recent", label: "Agent • recent" }),
          profileOption({ id: "p-other", label: "Agent • other" }),
        ]}
      />,
    );

    expect(
      screen.getAllByTestId(/option-/).map((option) => option.getAttribute("data-value")),
    ).toEqual(["p-recent", "p-unseen", "p-other"]);
  });
});

describe("useAgentProfileOptions executor-authoritative model hint", () => {
  it("keeps a profile whose start model is absent from the host probe selectable", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(getModelProbeWarningLabel()).toContain(GONE_MODEL);
  });

  it("keeps a profile with an available start model selectable", () => {
    const option = renderOptions([profileOption({ model: "gpt-5" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });

  it("keeps a profile with an empty (agent default) model selectable", () => {
    const option = renderOptions([profileOption({ model: "" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });

  it("keeps a gone-model profile with a fallback selectable", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL, fallback_model: "gpt-5" })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(option.getAttribute("data-reason")).toBeNull();

    expect(getModelProbeWarningLabel()).toContain(GONE_MODEL);
  });

  it("keeps a profile selectable when both saved models are absent from the host probe", () => {
    const option = renderOptions([
      profileOption({ model: GONE_MODEL, fallback_model: "other-gone" }),
    ]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(getModelProbeWarningLabel()).toContain(GONE_MODEL);
  });

  it("keeps auto-fallback profiles selectable and shows the host probe hint", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL, auto_fallback: true })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
    expect(getModelProbeWarningLabel()).toContain(GONE_MODEL);
  });

  it("renders the host probe hint as one compact warning trigger", () => {
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);

    expect(getModelProbeWarning()).toBeTruthy();
    expect(option.textContent).not.toContain(
      "The host probe did not advertise claude-gone. The selected executor will decide the model at launch.",
    );
  });

  it("uses a non-interactive warning indicator in the selected trigger", () => {
    const { result } = renderHook(() =>
      useAgentProfileOptions([profileOption({ model: GONE_MODEL })]),
    );

    render(
      <TooltipProvider>
        <div>{result.current[0]?.renderTriggerLabel?.()}</div>
      </TooltipProvider>,
    );

    expect(screen.queryByTestId(MODEL_PROBE_WARNING_TEST_ID)).toBeNull();
    expect(screen.getByTitle(/claude-gone/)).toBeTruthy();
  });

  it("does not gate when the agent model list is unknown (probe not landed)", () => {
    setAvailableAgents([]);
    const option = renderOptions([profileOption({ model: GONE_MODEL })]);
    expect(option.getAttribute(DATA_DISABLED)).toBeNull();
  });
});
