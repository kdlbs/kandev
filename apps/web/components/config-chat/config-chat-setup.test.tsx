import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfigChatSetup } from "./config-chat-setup";

const profiles = [
  { id: "profile-config", label: "Config Agent", agent_name: "codex" },
  { id: "profile-other", label: "Other Agent", agent_name: "claude" },
];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ agentProfiles: { items: profiles } }),
}));

afterEach(cleanup);

describe("ConfigChatSetup", () => {
  it("shows config-specific guidance and suggestions without repository controls", () => {
    render(
      <ConfigChatSetup
        defaultProfileId="profile-config"
        isStarting={false}
        error={null}
        onStart={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Configuration Chat" })).toBeTruthy();
    expect(
      screen.getByText(/manage workflows, agent profiles, and MCP configuration/i),
    ).toBeTruthy();
    expect(screen.getByPlaceholderText("Ask anything about your configuration...")).toBeTruthy();
    expect(screen.queryByText(/repositories/i)).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Show me the current workflow configuration" }),
    );
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe(
      "Show me the current workflow configuration",
    );
  });

  it("requires a configuration profile before showing the prompt composer", () => {
    render(
      <ConfigChatSetup isStarting={false} error={null} onStart={vi.fn()} onCancel={vi.fn()} />,
    );

    expect(screen.queryByPlaceholderText("Ask anything about your configuration...")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Config Agent codex/i }));
    expect(screen.getByPlaceholderText("Ask anything about your configuration...")).toBeTruthy();
  });

  it("ignores repeated and composing Enter keydowns", () => {
    const onStart = vi.fn();
    render(
      <ConfigChatSetup
        defaultProfileId="profile-config"
        isStarting={false}
        error={null}
        onStart={onStart}
        onCancel={vi.fn()}
      />,
    );
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "Update my workflow" } });

    fireEvent.keyDown(input, { key: "Enter", repeat: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229, isComposing: true });

    expect(onStart).not.toHaveBeenCalled();
  });
});
