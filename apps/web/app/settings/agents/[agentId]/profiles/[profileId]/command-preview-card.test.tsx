import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { CommandPreviewCard } from "./command-preview-card";
import type { CommandPreviewResponse } from "@/app/actions/agents";

const previewAgentCommandActionMock = vi.fn();

vi.mock("@/app/actions/agents", () => ({
  previewAgentCommandAction: (...args: unknown[]) => previewAgentCommandActionMock(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function response(commandString: string): CommandPreviewResponse {
  return {
    supported: true,
    command: [],
    command_string: "",
    composition: {
      wrapper: null,
      agent: { executable: commandString, omitted_arguments: 0, references: [] },
    },
  };
}

describe("CommandPreviewCard", () => {
  it("ignores a stale response that resolves after a newer request", async () => {
    // The initial render's request is left pending (simulating a slow
    // network response for the "old" settings). Once it has actually fired
    // (past the 300ms debounce), the settings change so a second, newer
    // request goes out and resolves quickly. The stale first response must
    // not overwrite the newer preview once it eventually resolves.
    let resolveStale: (value: CommandPreviewResponse) => void = () => {};
    const stalePromise = new Promise<CommandPreviewResponse>((resolve) => {
      resolveStale = resolve;
    });
    previewAgentCommandActionMock.mockImplementationOnce(() => stalePromise);

    const { rerender } = render(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough={false}
        cliFlags={[]}
        commandPrefix=""
      />,
    );

    fireEvent.click(screen.getByText(commandPreviewLabel));
    // Wait past the debounce so the stale request has actually fired.
    await waitFor(() => expect(previewAgentCommandActionMock).toHaveBeenCalledTimes(1));

    previewAgentCommandActionMock.mockImplementationOnce(async () => response("agent-new"));
    rerender(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough={false}
        cliFlags={[]}
        commandPrefix="greywall --"
      />,
    );

    await waitFor(() => expect(screen.getByText("agent-new")).toBeTruthy());

    // The stale request finally resolves — it must not clobber the newer preview.
    await act(async () => {
      resolveStale(response("agent-stale"));
      await stalePromise;
    });

    await waitFor(() => {
      expect(screen.getByText("agent-new")).toBeTruthy();
      expect(screen.queryByText("agent-stale")).not.toBeTruthy();
    });
  });

  it("requests a safe composition only after opening diagnostics", async () => {
    previewAgentCommandActionMock.mockResolvedValue({
      ...response("agent"),
      command: ["opaque-secret"],
      command_string: "opaque-secret",
    });
    render(
      <CommandPreviewCard
        agentName="agent"
        model="model"
        permissionSettings={{}}
        cliPassthrough
        cliFlags={[]}
        commandPrefix="wrapper --token=opaque-secret"
      />,
    );
    expect(previewAgentCommandActionMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText(commandPreviewLabel));
    await screen.findByText("agent");
    expect(previewAgentCommandActionMock.mock.calls[0][1].safe_only).toBe(true);
    expect(screen.queryByText(/opaque-secret/)).toBeNull();
  });
});

describe("CommandPreviewCard flag destination", () => {
  // AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.2, .4
  // Over ACP the flags land on the bridge process, not the agent CLI it wraps.
  // Without this line the preview showed an argv that looks like the agent's.
  it("names the launched process when flags go to the ACP bridge", async () => {
    previewAgentCommandActionMock.mockResolvedValue({
      ...response("npx --yes @agentclientprotocol/claude-agent-acp --verbose"),
      flag_destination: "acp_bridge",
    });

    render(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough={false}
        cliFlags={[{ description: "", flag: "--verbose", enabled: true }]}
        commandPrefix=""
      />,
    );

    fireEvent.click(screen.getByText(commandPreviewLabel));
    await waitFor(() => {
      expect(screen.getByTestId("command-preview-flag-destination")).toBeTruthy();
    });
  });

  it("omits the destination note for a passthrough profile", async () => {
    previewAgentCommandActionMock.mockResolvedValue({
      ...response("npx -y @anthropic-ai/claude-code --verbose"),
      flag_destination: "agent_cli",
    });

    render(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough
        cliFlags={[{ description: "", flag: "--verbose", enabled: true }]}
        commandPrefix=""
      />,
    );

    fireEvent.click(screen.getByText(commandPreviewLabel));
    await waitFor(() => {
      expect(screen.queryByTestId("command-preview-flag-destination")).toBeNull();
    });
  });
});

describe("CommandPreviewCard deferred Copilot mounts", () => {
  it("shows deferred plugin identities without a resolved directory", async () => {
    previewAgentCommandActionMock.mockImplementation(async () => ({
      ...response("npx @github/copilot@1.0.88"),
      deferred_plugin_ids: ["installed:copilot-plugins/spark"],
      deferred_plugin_count: 1,
      command_incomplete: true,
    }));

    render(
      <CommandPreviewCard
        agentName="copilot-acp"
        model="auto"
        permissionSettings={{}}
        cliPassthrough
        cliFlags={[]}
        commandPrefix=""
        copilotSelectedPlugins={["installed:copilot-plugins/spark"]}
      />,
    );

    fireEvent.click(screen.getByText(commandPreviewLabel));
    expect(await screen.findByTestId("command-preview-deferred-plugins")).toBeTruthy();
    expect(screen.getByText(/Selected plugin mounts resolve/)).toBeTruthy();
    expect(screen.getByText(/installed:copilot-plugins\/spark/)).toBeTruthy();
    expect(screen.getByText(/Base command is incomplete/)).toBeTruthy();
    expect(screen.queryByText(/--plugin-dir/)).toBeNull();
    expect(previewAgentCommandActionMock).toHaveBeenCalledWith(
      "copilot-acp",
      expect.objectContaining({
        copilot_selected_plugins: ["installed:copilot-plugins/spark"],
      }),
    );
  });
});
const commandPreviewLabel = "Command Preview";
