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

describe("useAgentProfileOptions model-independent labels", () => {
  // @covers AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.1
  it.each([
    ["exact", "gpt-5", ["gpt-5"]],
    ["missing", GONE_MODEL, ["gpt-5"]],
    ["unique variation", "opus", ["opus[1m]"]],
    ["multiple variations", "opus", ["opus[1m]", "opus[270k]"]],
    ["legacy effort IDs", "gpt-6-astra", ["gpt-6-astra[low]", "gpt-6-astra[high]"]],
    ["bracketed request", "opus[1m]", ["opus[270k]"]],
    ["empty catalog", GONE_MODEL, []],
    ["provider default", "", ["gpt-5"]],
  ])("does not show host model advisories in either label: %s", (_, model, models) => {
    setAvailableAgents([
      {
        ...AGENT_WITH_GPT,
        model_config: {
          ...AGENT_WITH_GPT.model_config,
          available_models: (models as string[]).map((id) => ({ id, name: id })),
        },
      },
    ]);
    const profile = profileOption({ model: model as string });
    const { result } = renderHook(() => useAgentProfileOptions([profile]));
    const option = result.current[0]!;
    expect(option.disabled).toBeUndefined();
    expect(option.disabledReason).toBeUndefined();
    render(
      <TooltipProvider>
        <div data-testid="option-label">{option.renderLabel()}</div>
        <div data-testid="selected-label">{option.renderTriggerLabel?.()}</div>
      </TooltipProvider>,
    );
    for (const label of ["option-label", "selected-label"]) {
      const element = screen.getByTestId(label);
      expect(element.textContent).toContain("hybrid");
      expect(element.querySelector("button, .tabler-icon-alert-triangle")).toBeNull();
      expect(element.querySelector('[title*="host probe"]')).toBeNull();
    }
  });

  // @covers AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.3
  it.each(["auth_required", "not_installed", "failed"] as const)(
    "preserves %s health indicators when the saved model is absent",
    (capability_status) => {
      const profile = profileOption({
        model: GONE_MODEL,
        capability_status,
        capability_error: "Agent needs attention",
      });
      const { result } = renderHook(() => useAgentProfileOptions([profile]));
      const option = result.current[0]!;
      render(
        <TooltipProvider>
          <div>{option.renderLabel()}</div>
          <div>{option.renderTriggerLabel?.()}</div>
        </TooltipProvider>,
      );
      expect(screen.getAllByTitle("Agent needs attention")).toHaveLength(2);
      expect(screen.queryByTestId(MODEL_PROBE_WARNING_TEST_ID)).toBeNull();
    },
  );

  // @covers AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.6
  it.each([false, true])(
    "preserves saved profile settings with auto fallback %s",
    (auto_fallback) => {
      const profile = profileOption({
        model: GONE_MODEL,
        fallback_model: "other-gone",
        auto_fallback,
        cli_passthrough: true,
      });
      const saved = structuredClone(profile);
      const option = renderOptions([profile]);
      expect(option.getAttribute(DATA_DISABLED)).toBeNull();
      expect(option.getAttribute("data-reason")).toBeNull();
      expect(screen.queryByTestId(MODEL_PROBE_WARNING_TEST_ID)).toBeNull();
      expect(option.querySelector(".tabler-icon-terminal-2")).not.toBeNull();
      expect(profile).toEqual(saved);
    },
  );

  it("keeps labels stable when a pending host catalog changes", () => {
    setAvailableAgents([]);
    const profile = profileOption({ model: GONE_MODEL });
    const { rerender } = render(<OptionsProbe profiles={[profile]} />);
    const initialLabel = screen.getByTestId("option-0").innerHTML;
    setAvailableAgents([AGENT_WITH_GPT]);
    rerender(<OptionsProbe profiles={[profile]} />);
    expect(screen.getByTestId("option-0").innerHTML).toBe(initialLabel);
    expect(screen.queryByTestId(MODEL_PROBE_WARNING_TEST_ID)).toBeNull();
  });
});
