import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { McpPolicyCard } from "./mcp-policy-card";
import { ProfileDetailsCard } from "./profile-details-card";

afterEach(cleanup);
const MCP_POLICY_LABEL = "MCP policy JSON";
const DIRTY_ATTRIBUTE = "data-settings-dirty";

describe("executor profile dirty fields", () => {
  it("marks a changed profile name and its owning card dirty", () => {
    render(<ProfileDetailsCard name="Edited" baselineName="Saved" onNameChange={vi.fn()} />);

    const input = screen.getByLabelText("Name");
    expect(input.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(input.closest('[data-slot="card"]')?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });

  it("keeps the MCP policy clean until its value changes", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <McpPolicyCard
        mcpPolicy='{"allow_http":true}'
        baselinePolicy='{"allow_http":true}'
        mcpPolicyError={null}
        onPolicyChange={onChange}
      />,
    );

    expect(screen.getByLabelText(MCP_POLICY_LABEL).getAttribute(DIRTY_ATTRIBUTE)).toBe("false");

    rerender(
      <McpPolicyCard
        mcpPolicy='{"allow_http":false}'
        baselinePolicy='{"allow_http":true}'
        mcpPolicyError={null}
        onPolicyChange={onChange}
      />,
    );
    expect(screen.getByLabelText(MCP_POLICY_LABEL).getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });

  it("continues to report field changes through the supplied callback", () => {
    const onNameChange = vi.fn();
    render(<ProfileDetailsCard name="Saved" baselineName="Saved" onNameChange={onNameChange} />);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Edited" } });
    expect(onNameChange).toHaveBeenCalledWith("Edited");
  });
});
