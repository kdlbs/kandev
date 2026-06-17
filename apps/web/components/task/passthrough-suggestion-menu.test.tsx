import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PassthroughSuggestionMenu } from "./passthrough-suggestion-menu";

describe("PassthroughSuggestionMenu", () => {
  it("announces suggestions as a selectable listbox", () => {
    render(
      <PassthroughSuggestionMenu
        open
        suggestion={{ kind: "command", triggerStart: 0, query: "re" }}
        items={[
          {
            id: "agent-review",
            label: "/review",
            description: "Review changes",
            agentCommandName: "review",
          },
          {
            id: "agent-resume",
            label: "/resume",
            description: "Resume task",
            agentCommandName: "resume",
          },
        ]}
        selectedIndex={1}
        setSelectedIndex={vi.fn()}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByRole("listbox", { name: "Command suggestions" })).not.toBeNull();
    expect(screen.getByRole("option", { name: "/review" }).getAttribute("aria-selected")).toBe(
      "false",
    );
    expect(screen.getByRole("option", { name: "/resume" }).getAttribute("aria-selected")).toBe(
      "true",
    );
  });
});
