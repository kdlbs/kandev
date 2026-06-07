import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { InferenceAgentStatusNote } from "./inference-agent-status";
import type { InferenceAgent, InferenceAgentStatus } from "@/lib/api/domains/utility-api";

afterEach(() => {
  cleanup();
});

function makeAgent(overrides: Partial<InferenceAgent> = {}): InferenceAgent {
  return {
    id: "claude-acp",
    name: "Claude ACP Agent",
    display_name: "Claude",
    models: [],
    status: "ok",
    ...overrides,
  };
}

describe("InferenceAgentStatusNote", () => {
  it("renders nothing when agent is healthy with models", () => {
    const { container } = render(
      <InferenceAgentStatusNote
        agent={makeAgent({
          status: "ok",
          models: [{ id: "sonnet", name: "Sonnet", description: "", is_default: true }],
        })}
        onRefresh={() => {}}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it.each<[InferenceAgentStatus, string]>([
    ["probing", "Setting up Claude"],
    ["auth_required", "Sign in to Claude"],
    ["not_installed", "Claude CLI is not installed"],
    ["not_configured", "Claude is not configured"],
    ["failed", "Probe failed for Claude"],
  ])("maps status %s to the expected note copy", (status, expectedSubstring) => {
    render(<InferenceAgentStatusNote agent={makeAgent({ status })} onRefresh={() => {}} />);
    expect(screen.getByText(new RegExp(expectedSubstring))).toBeTruthy();
  });

  it("treats ok-with-no-models as an empty-advertised-models state", () => {
    render(
      <InferenceAgentStatusNote
        agent={makeAgent({ status: "ok", models: [] })}
        onRefresh={() => {}}
      />,
    );
    expect(screen.getByText(/Claude advertised no models/)).toBeTruthy();
  });

  it("renders fallback copy when agent is missing from the list", () => {
    render(
      <InferenceAgentStatusNote agent={null} fallbackName="claude-acp" onRefresh={() => {}} />,
    );
    expect(screen.getByText(/claude-acp is no longer available/)).toBeTruthy();
  });

  it("hides the Refresh button for non-refreshable statuses", () => {
    render(
      <InferenceAgentStatusNote
        agent={makeAgent({ status: "not_configured" })}
        onRefresh={() => {}}
      />,
    );
    expect(screen.queryByTestId("inference-agent-refresh")).toBeNull();
  });

  it("invokes onRefresh and disables the button while refreshing", async () => {
    let resolve: (() => void) | undefined;
    const onRefresh = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve = r;
        }),
    );
    render(
      <InferenceAgentStatusNote
        agent={makeAgent({ status: "auth_required" })}
        onRefresh={onRefresh}
      />,
    );
    const button = screen.getByTestId("inference-agent-refresh") as HTMLButtonElement;
    fireEvent.click(button);
    expect(onRefresh).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(button.disabled).toBe(true);
    });
    resolve?.();
    await waitFor(() => {
      expect(button.disabled).toBe(false);
    });
  });

  it("exposes the sanitized status_message detail line", () => {
    render(
      <InferenceAgentStatusNote
        agent={makeAgent({
          status: "failed",
          status_message: "subprocess exited with code 1",
        })}
        onRefresh={() => {}}
      />,
    );
    expect(screen.getByText("subprocess exited with code 1")).toBeTruthy();
  });
});
