import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
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
    await act(async () => {
      resolveStale(response("agent --stale"));
      await stalePromise;
    });

    await waitFor(() => {
      expect(screen.getByText("agent --new")).toBeTruthy();
      expect(screen.queryByText("agent --stale")).not.toBeTruthy();
    });
  });

  // The `{prompt}` hint is a <Trans>: its <0> index addresses the JSX children
  // positionally, so a prettier reflow can silently reassemble the sentence
  // into fragments without failing anything. Assert the whole reconstructed
  // sentence, and that the token itself is never translated or re-interpolated.
  it("renders the {prompt} hint as one sentence with the token intact", async () => {
    previewAgentCommandActionMock.mockImplementation(async () => response("agent run {prompt}"));

    render(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough={false}
        cliFlags={[]}
        commandPrefix=""
      />,
    );

    const hint = await screen.findByText(
      (_content, element) =>
        element?.tagName === "P" &&
        element.textContent ===
          "{prompt} will be replaced with your task description or follow-up message.",
    );
    expect(hint.querySelector("code")?.textContent).toBe("{prompt}");
  });

  it("omits the {prompt} hint in CLI passthrough mode", async () => {
    previewAgentCommandActionMock.mockImplementation(async () => response("agent"));

    render(
      <CommandPreviewCard
        agentName="claude"
        model="claude-sonnet-4-5"
        permissionSettings={{}}
        cliPassthrough
        cliFlags={[]}
        commandPrefix=""
      />,
    );

    await waitFor(() => expect(screen.getByText("agent")).toBeTruthy());
    expect(screen.queryByText(/will be replaced with your task description/)).toBeNull();
  });
});
