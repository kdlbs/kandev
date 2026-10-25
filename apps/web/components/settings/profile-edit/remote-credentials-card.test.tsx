import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RemoteCredentialsCard } from "./remote-credentials-card";
import { AgentConfigOptions } from "./portable-config-bundles";
import { listRemoteCredentials } from "@/lib/api/domains/settings-api";
import { listAgentConfigBundles, type AgentConfigBundle } from "@/lib/api/domains/agent-config-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

vi.mock("@/lib/api/domains/settings-api", () => ({
  listRemoteCredentials: vi.fn(),
}));

vi.mock("@/lib/api/domains/agent-config-api", () => ({
  listAgentConfigBundles: vi.fn(),
}));

const codexFileMethodId = "agent:codex-acp:files:0";
const codexAgentName = "Codex";
const copyAuthFilesLabel = "Copy auth files";
const provideSecretLabel = "Provide secret";
const codexConfigBundleId = "codex.config";
const copyCodexConfigLabel = "Copy Codex configuration";
const CHECKED_STATE = "checked";
const UNCHECKED_STATE = "unchecked";
const TRUE_ATTRIBUTE = "true";
const DATA_STATE_ATTRIBUTE = "data-state";

function renderRemoteCredentialsCard(
  onChange = vi.fn(),
  onConfigChange = vi.fn(),
  initialConfigBundleIds: string[] = [],
  options: {
    agentEnvVars?: Record<string, string | null>;
    onAgentEnvVarChange?: (methodId: string, secretId: string | null) => void;
    secrets?: SecretListItem[];
  } = {},
) {
  render(
    <RemoteCredentialsHarness
      onChange={onChange}
      onConfigChange={onConfigChange}
      initialConfigBundleIds={initialConfigBundleIds}
      agentEnvVars={options.agentEnvVars}
      onAgentEnvVarChange={options.onAgentEnvVarChange}
      secrets={options.secrets}
    />,
  );
}

function RemoteCredentialsHarness({
  onChange,
  onConfigChange,
  initialConfigBundleIds,
  agentEnvVars: initialAgentEnvVars = {},
  onAgentEnvVarChange: onEnvVarChange = () => {},
  secrets = [],
}: {
  onChange: (ids: string[]) => void;
  onConfigChange: (ids: string[]) => void;
  initialConfigBundleIds: string[];
  agentEnvVars?: Record<string, string | null>;
  onAgentEnvVarChange?: (methodId: string, secretId: string | null) => void;
  secrets?: SecretListItem[];
}) {
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [configBundleIds, setConfigBundleIds] = useState(initialConfigBundleIds);
  const [agentEnvVars, setAgentEnvVars] = useState(initialAgentEnvVars);
  return (
    <RemoteCredentialsCard
      isRemote={true}
      selectedIds={selectedIds}
      onChange={(ids) => {
        setSelectedIds(ids);
        onChange(ids);
      }}
      configBundleIds={configBundleIds}
      onConfigBundleChange={(ids) => {
        setConfigBundleIds(ids);
        onConfigChange(ids);
      }}
      agentEnvVars={agentEnvVars}
      onAgentEnvVarChange={(methodId, secretId) => {
        setAgentEnvVars((current) => ({ ...current, [methodId]: secretId }));
        onEnvVarChange(methodId, secretId);
      }}
      secrets={secrets}
      gitIdentityMode="override"
      onGitIdentityModeChange={() => {}}
      gitUserName=""
      gitUserEmail=""
      onGitUserNameChange={() => {}}
      onGitUserEmailChange={() => {}}
      localGitIdentity={{ userName: "", userEmail: "", detected: false }}
    />
  );
}

beforeEach(() => {
  vi.mocked(listAgentConfigBundles).mockResolvedValue({ bundles: [] });
  vi.mocked(listRemoteCredentials).mockResolvedValue({
    auth_specs: [
      {
        id: "codex-acp",
        display_name: codexAgentName,
        methods: [
          {
            method_id: codexFileMethodId,
            type: "files",
            label: copyAuthFilesLabel,
            source_files: [".codex/auth.json", ".codex/config.toml"],
            has_local_files: false,
          },
          {
            method_id: "agent:codex-acp:env:OPENAI_API_KEY",
            type: "env",
            env_var: "OPENAI_API_KEY",
          },
        ],
      },
    ],
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RemoteCredentialsCard", () => {
  it("does not crash when an auth spec has no methods", async () => {
    vi.mocked(listRemoteCredentials).mockResolvedValueOnce({
      auth_specs: [
        {
          id: "empty-agent",
          display_name: "Empty Agent",
          methods: null as never,
        },
      ],
    });

    renderRemoteCredentialsCard();

    expect(await screen.findByText("Empty Agent")).toBeTruthy();
  });

  it("allows selecting file auth even when local file detection reports missing files", async () => {
    const onChange = vi.fn();
    renderRemoteCredentialsCard(onChange);

    fireEvent.click(await screen.findByText(codexAgentName));
    const fileOption = screen.getByRole("checkbox", { name: copyAuthFilesLabel });
    expect(fileOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);

    fireEvent.click(fileOption);

    expect(onChange).toHaveBeenCalledWith([codexFileMethodId]);
    await waitFor(() => {
      expect(
        screen
          .getByRole("checkbox", { name: copyAuthFilesLabel })
          .getAttribute(DATA_STATE_ATTRIBUTE),
      ).toBe(CHECKED_STATE);
    });
  });

  it("marks the selected credential and owning card dirty", async () => {
    renderRemoteCredentialsCard();

    fireEvent.click(await screen.findByText(codexAgentName));
    const fileOption = screen.getByRole("checkbox", { name: copyAuthFilesLabel });
    fireEvent.click(fileOption);

    await waitFor(() => {
      expect(fileOption.closest("label")?.getAttribute("data-settings-dirty")).toBe(TRUE_ATTRIBUTE);
      expect(
        screen
          .getByText("Remote Credentials")
          .closest("[data-settings-dirty]")
          ?.getAttribute("data-settings-dirty"),
      ).toBe(TRUE_ATTRIBUTE);
    });
  });

  // The `<Trans>` in EnvOption addresses its JSX children positionally, so a
  // prettier reflow can silently reassemble the sentence into fragments with an
  // empty <code>. Assert the whole reconstructed sentence, not just a fragment.
  it("renders the env-var secret hint as one sentence with the variable inline", async () => {
    renderRemoteCredentialsCard();

    fireEvent.click(await screen.findByText(codexAgentName));
    const envOption = screen.getByRole("checkbox", { name: provideSecretLabel });

    const envOptionLabel = envOption.closest("label");
    expect(envOptionLabel?.textContent).toContain("Set OPENAI_API_KEY via a stored secret");
    expect(envOptionLabel?.querySelector("code")?.textContent).toBe("OPENAI_API_KEY");
  });

  it("appends the not-found note to the auth file list as one string", async () => {
    renderRemoteCredentialsCard();

    fireEvent.click(await screen.findByText(codexAgentName));

    expect(
      screen.getByText(".codex/auth.json, .codex/config.toml; files not found on this machine"),
    ).toBeTruthy();
  });

  it("places configuration choices inside the matching agent accordion", async () => {
    vi.mocked(listAgentConfigBundles).mockResolvedValueOnce({
      bundles: [PORTABLE_BUNDLES[1]],
    });

    renderRemoteCredentialsCard();

    await screen.findByText(codexAgentName);
    await waitFor(() => {
      expect(screen.queryByRole("checkbox", { name: copyCodexConfigLabel })).toBeNull();
    });

    fireEvent.click(screen.getByText(codexAgentName));

    const options = await screen.findByTestId("agent-config-options-codex-acp");
    expect(options).toBeTruthy();
    expect(screen.queryByText("Agent configuration")).toBeNull();
    expect(options.contains(screen.getByRole("checkbox", { name: copyCodexConfigLabel }))).toBe(
      true,
    );
  });

  it("keeps authentication and configuration selections independent inside one agent", async () => {
    vi.mocked(listAgentConfigBundles).mockResolvedValueOnce({
      bundles: [{ ...PORTABLE_BUNDLES[1], available: true }],
    });
    const onAuthChange = vi.fn();
    const onConfigChange = vi.fn();

    renderRemoteCredentialsCard(onAuthChange, onConfigChange);
    fireEvent.click(await screen.findByText(codexAgentName));

    const authOption = screen.getByRole("checkbox", { name: copyAuthFilesLabel });
    fireEvent.click(authOption);
    const configOptions = await screen.findByTestId("agent-config-options-codex-acp");
    fireEvent.click(within(configOptions).getByRole("checkbox", { name: copyCodexConfigLabel }));

    expect(authOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(onAuthChange).toHaveBeenCalledWith([codexFileMethodId]);
    expect(onConfigChange).toHaveBeenCalledWith([codexConfigBundleId]);
  });
});

describe("RemoteCredentialsCard auth selections", () => {
  it("allows file and secret authentication methods at the same time", async () => {
    const onAuthChange = vi.fn();
    renderRemoteCredentialsCard(onAuthChange);

    fireEvent.click(await screen.findByText(codexAgentName));
    const fileOption = screen.getByRole("checkbox", { name: copyAuthFilesLabel });
    const secretOption = screen.getByRole("checkbox", { name: provideSecretLabel });

    fireEvent.click(fileOption);
    fireEvent.click(secretOption);

    expect(fileOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(secretOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(onAuthChange).toHaveBeenLastCalledWith([
      codexFileMethodId,
      "agent:codex-acp:env:OPENAI_API_KEY",
    ]);
  });
});

describe("RemoteCredentialsCard with multiple env auth methods", () => {
  const antigravityAgentName = "Antigravity";
  const geminiMethodId = "agent:antigravity-acp:env:GEMINI_API_KEY";
  const googleMethodId = "agent:antigravity-acp:env:GOOGLE_API_KEY";
  const secrets: SecretListItem[] = [
    {
      id: "secret-gemini",
      name: "Gemini key",
      has_value: true,
      created_at: "",
      updated_at: "",
    },
    {
      id: "secret-google",
      name: "Google key",
      has_value: true,
      created_at: "",
      updated_at: "",
    },
  ];

  beforeEach(() => {
    vi.mocked(listRemoteCredentials).mockResolvedValue({
      auth_specs: [
        {
          id: "antigravity-acp",
          display_name: antigravityAgentName,
          methods: [
            { method_id: geminiMethodId, type: "env", env_var: "GEMINI_API_KEY" },
            { method_id: googleMethodId, type: "env", env_var: "GOOGLE_API_KEY" },
          ],
        },
      ],
    });
  });

  it("lets both declared env methods be selected independently", async () => {
    const onAuthChange = vi.fn();
    const onEnvVarChange = vi.fn();
    renderRemoteCredentialsCard(onAuthChange, vi.fn(), [], {
      onAgentEnvVarChange: onEnvVarChange,
      secrets,
    });

    fireEvent.click(await screen.findByText(antigravityAgentName));
    const geminiOption = screen.getByRole("checkbox", {
      name: "Provide secret for GEMINI_API_KEY",
    });
    const googleOption = screen.getByRole("checkbox", {
      name: "Provide secret for GOOGLE_API_KEY",
    });

    fireEvent.click(geminiOption);
    fireEvent.click(googleOption);

    const secretSelects = await screen.findAllByRole("combobox");
    expect(secretSelects).toHaveLength(2);

    fireEvent.click(secretSelects[0]);
    fireEvent.click(await screen.findByRole("option", { name: "Gemini key" }));
    fireEvent.click(secretSelects[1]);
    fireEvent.click(await screen.findByRole("option", { name: "Google key" }));

    expect(geminiOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(googleOption.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(onAuthChange).toHaveBeenLastCalledWith([geminiMethodId, googleMethodId]);
    expect(onEnvVarChange).toHaveBeenNthCalledWith(1, geminiMethodId, "secret-gemini");
    expect(onEnvVarChange).toHaveBeenNthCalledWith(2, googleMethodId, "secret-google");
  });
});

const PORTABLE_BUNDLES: AgentConfigBundle[] = [
  {
    id: "claude.settings",
    agent_id: "claude-acp",
    display_name: "Claude settings",
    label: "Claude settings",
    available: true,
    files: [
      {
        source_path: ".claude/settings.json",
        target_path: ".claude/settings.json",
        available: true,
      },
    ],
  },
  {
    id: codexConfigBundleId,
    agent_id: "codex-acp",
    display_name: "Codex configuration",
    label: "Codex configuration",
    available: false,
    files: [
      { source_path: ".codex/config.toml", target_path: ".codex/config.toml", available: false },
    ],
  },
];

describe("AgentConfigOptions", () => {
  it("keeps configuration selection independent from authentication", () => {
    const onChange = vi.fn();

    const { rerender } = render(
      <AgentConfigOptions
        agentId="codex-acp"
        bundles={PORTABLE_BUNDLES}
        selectedIds={[]}
        baselineSelectedIds={[]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole("checkbox", { name: "Copy Claude settings" }));
    rerender(
      <AgentConfigOptions
        agentId="codex-acp"
        bundles={PORTABLE_BUNDLES}
        selectedIds={["claude.settings"]}
        baselineSelectedIds={[]}
        onChange={onChange}
      />,
    );

    expect(onChange).toHaveBeenCalledWith(["claude.settings"]);
    expect(
      screen.getByTestId("agent-config-options-codex-acp").getAttribute("data-settings-dirty"),
    ).toBe(TRUE_ATTRIBUTE);
  });

  it("disables new unavailable bundles but keeps saved ones removable", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <AgentConfigOptions
        agentId="codex-acp"
        bundles={PORTABLE_BUNDLES}
        selectedIds={[]}
        baselineSelectedIds={[]}
        onChange={onChange}
      />,
    );

    const checkbox = screen.getByRole("checkbox", { name: copyCodexConfigLabel });
    expect((checkbox as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByText("Not found")).toBeTruthy();
    expect(screen.getByText(/editing hint/i)).toBeTruthy();

    rerender(
      <AgentConfigOptions
        agentId="codex-acp"
        bundles={PORTABLE_BUNDLES}
        selectedIds={[codexConfigBundleId]}
        baselineSelectedIds={[codexConfigBundleId]}
        onChange={onChange}
      />,
    );
    const savedCheckbox = screen.getByRole("checkbox", { name: copyCodexConfigLabel });
    expect((savedCheckbox as HTMLInputElement).disabled).toBe(false);
    fireEvent.click(savedCheckbox);
    expect(onChange).toHaveBeenCalledWith([]);
    rerender(
      <AgentConfigOptions
        agentId="codex-acp"
        bundles={PORTABLE_BUNDLES}
        selectedIds={[]}
        baselineSelectedIds={[]}
        onChange={onChange}
      />,
    );
  });
});
