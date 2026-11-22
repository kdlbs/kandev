import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({ count: 0 }));
vi.mock("@/hooks/domains/settings/use-prompt-conflict-count", () => ({
  usePromptConflictCount: () => state.count,
}));

import { PromptConflictIndicator } from "./prompt-conflict-indicator";

afterEach(cleanup);

// @covers AC-INTEGRATIONS-PROMPT-SYNC-008.5
describe("PromptConflictIndicator", () => {
  it("renders an announced dot only while a conflict is unresolved", () => {
    state.count = 0;
    const { rerender } = render(<PromptConflictIndicator />);
    expect(screen.queryByTestId("prompt-conflict-indicator")).toBeNull();
    state.count = 2;
    rerender(<PromptConflictIndicator />);
    const dot = screen.getByRole("img", { name: "Prompt conflicts need review" });
    expect(dot.getAttribute("data-testid")).toBe("prompt-conflict-indicator");
  });
});
