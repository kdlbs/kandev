import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RemoteCredentialsCard } from "./remote-credentials-card";
import { listRemoteCredentials } from "@/lib/api/domains/settings-api";

vi.mock("@/lib/api/domains/settings-api", () => ({
  listRemoteCredentials: vi.fn(),
}));

const codexFileMethodId = "agent:codex-acp:files:0";
const copyAuthFilesLabel = "Copy auth files";

function renderRemoteCredentialsCard(onChange = vi.fn()) {
  render(<RemoteCredentialsHarness onChange={onChange} />);
}

function RemoteCredentialsHarness({ onChange }: { onChange: (ids: string[]) => void }) {
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  return (
    <RemoteCredentialsCard
      isRemote={true}
      selectedIds={selectedIds}
      onChange={(ids) => {
        setSelectedIds(ids);
        onChange(ids);
      }}
      agentEnvVars={{}}
      onAgentEnvVarChange={() => {}}
      secrets={[]}
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
  vi.mocked(listRemoteCredentials).mockResolvedValue({
    auth_specs: [
      {
        id: "codex-acp",
        display_name: "Codex",
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

    fireEvent.click(await screen.findByText("Codex"));
    const fileOption = screen.getByRole("radio", { name: copyAuthFilesLabel });
    expect(fileOption.getAttribute("aria-checked")).toBe("false");

    fireEvent.click(fileOption);

    expect(onChange).toHaveBeenCalledWith([codexFileMethodId]);
    await waitFor(() => {
      const selectedOption = screen.getByRole("radio", { name: copyAuthFilesLabel });
      expect(selectedOption.getAttribute("aria-checked")).toBe("true");
    });
  });

  it("marks the selected credential and owning card dirty", async () => {
    renderRemoteCredentialsCard();

    fireEvent.click(await screen.findByText("Codex"));
    const fileOption = screen.getByRole("radio", { name: copyAuthFilesLabel });
    fireEvent.click(fileOption);

    await waitFor(() => {
      expect(fileOption.getAttribute("data-settings-dirty")).toBe("true");
      expect(
        screen
          .getByText("Remote Credentials")
          .closest("[data-settings-dirty]")
          ?.getAttribute("data-settings-dirty"),
      ).toBe("true");
    });
  });
});
