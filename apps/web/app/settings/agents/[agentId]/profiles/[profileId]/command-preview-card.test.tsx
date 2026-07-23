import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
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
  return { supported: true, command: commandString.split(" "), command_string: commandString };
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

    // Wait past the debounce so the stale request has actually fired.
    await waitFor(() => expect(previewAgentCommandActionMock).toHaveBeenCalledTimes(1));

    previewAgentCommandActionMock.mockImplementationOnce(async () => response("agent --new"));
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

    await waitFor(() => expect(screen.getByText("agent --new")).toBeTruthy());

    // The stale request finally resolves — it must not clobber the newer preview.
    resolveStale(response("agent --stale"));
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(screen.getByText("agent --new")).toBeTruthy();
    expect(screen.queryByText("agent --stale")).not.toBeTruthy();
  });
});
