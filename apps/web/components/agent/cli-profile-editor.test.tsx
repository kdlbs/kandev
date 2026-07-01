import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { CliProfileEditor } from "./cli-profile-editor";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const updateActionMock = vi.fn();
const createProfileActionMock = vi.fn();
const createAgentActionMock = vi.fn();
const MODEL_ID = "claude-sonnet-4-5";
const CREATED_AT = "2026-01-01T00:00:00Z";
const UPDATED_AT = "2026-05-04T00:00:00Z";

vi.mock("@/app/actions/agents", () => ({
  updateAgentProfileAction: (...args: unknown[]) => updateActionMock(...args),
  createAgentProfileAction: (...args: unknown[]) => createProfileActionMock(...args),
  createAgentAction: (...args: unknown[]) => createAgentActionMock(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const baseAvailableAgent = {
  name: "claude",
  display_name: "Claude",
  supports_mcp: true,
  installation_paths: [],
  available: true,
  capabilities: {
    supports_session_resume: true,
    supports_shell: true,
    supports_workspace_only: false,
  },
  model_config: {
    default_model: MODEL_ID,
    available_models: [
      { id: MODEL_ID, name: "Claude Sonnet 4.5" },
      { id: "claude-opus-4", name: "Claude Opus 4" },
    ],
    supports_dynamic_models: false,
  },
  permission_settings: {},
  updated_at: UPDATED_AT,
};

function renderWithSettings(element: ReactElement, availableAgents = [baseAvailableAgent]) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.settings.agents(), { agents: [], total: 0 });
  queryClient.setQueryData(qk.settings.availableAgents(), {
    agents: availableAgents,
    tools: [],
    total: availableAgents.length,
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={{}}>{element}</StateProvider>
    </QueryClientProvider>,
  );
}

const agentWithRecommendedFlag = {
  ...baseAvailableAgent,
  name: "copilot",
  display_name: "Copilot",
  permission_settings: {
    allow_all_tools: {
      key: "allow_all_tools",
      label: "Allow all tools",
      description: "Allow Copilot to use tools without prompting.",
      supported: true,
      default: true,
      apply_method: "cli_flag",
      cli_flag: "--allow-all-tools",
    },
  },
};

describe("CliProfileEditor", () => {
  it("renders the create form with the CLI client picker, profile name, and model selector", () => {
    renderWithSettings(
      <CliProfileEditor mode="create" defaultProfileName="default" onSaved={vi.fn()} />,
    );

    expect(screen.getByLabelText("Profile name")).toBeTruthy();
    expect(screen.getByText("Create profile")).toBeTruthy();
  });

  it("shows existing profile values in edit mode", () => {
    const profile = {
      id: toAgentProfileId("p1"),
      name: "default",
      agentId: "claude",
      agentDisplayName: "Claude",
      model: MODEL_ID,
      mode: "",
      allowIndexing: false,
      autoApprove: false,
      cliFlags: [],
      cliPassthrough: false,
      createdAt: CREATED_AT,
      updatedAt: CREATED_AT,
    };
    renderWithSettings(<CliProfileEditor mode="edit" profile={profile} onSaved={vi.fn()} />);

    const nameInput = screen.getByLabelText("Profile name") as HTMLInputElement;
    expect(nameInput.value).toBe("default");
    expect(screen.getByText("Save profile")).toBeTruthy();
  });

  it("invokes onCancel when the Cancel button is clicked", () => {
    const onCancel = vi.fn();
    renderWithSettings(<CliProfileEditor mode="create" onSaved={vi.fn()} onCancel={onCancel} />);

    fireEvent.click(screen.getByText("Cancel"));
    expect(onCancel).toHaveBeenCalled();
  });
});

describe("CliProfileEditor recommended flags", () => {
  it("can hide passthrough while showing recommended CLI flags", () => {
    renderWithSettings(
      <CliProfileEditor mode="create" showAdvanced allowCliPassthrough={false} onSaved={vi.fn()} />,
      [agentWithRecommendedFlag],
    );

    expect(screen.queryByText("CLI passthrough")).toBeNull();
    expect(screen.getByText("Allow all tools")).toBeTruthy();
  });

  it("saves seeded recommended CLI flags for new profiles", async () => {
    createAgentActionMock.mockResolvedValue({
      profiles: [
        {
          id: "profile-copilot",
          name: "default",
          agentId: "copilot",
          agentDisplayName: "Copilot",
          model: MODEL_ID,
          cliFlags: [],
          cliPassthrough: false,
          allowIndexing: false,
          autoApprove: false,
          createdAt: CREATED_AT,
          updatedAt: CREATED_AT,
        },
      ],
    });

    renderWithSettings(
      <CliProfileEditor mode="create" defaultProfileName="default" onSaved={vi.fn()} />,
      [agentWithRecommendedFlag],
    );

    fireEvent.click(screen.getByText("Create profile"));

    await waitFor(() => expect(createAgentActionMock).toHaveBeenCalled());
    expect(createAgentActionMock.mock.calls[0][0].profiles[0].cli_flags).toEqual([
      {
        description: "Allow Copilot to use tools without prompting.",
        enabled: true,
        flag: "--allow-all-tools",
      },
    ]);
  });
});
