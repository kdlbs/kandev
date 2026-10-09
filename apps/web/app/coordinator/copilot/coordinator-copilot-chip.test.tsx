import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CoordinatorCopilotChipRow,
  CoordinatorCopilotEmptyIntro,
} from "./coordinator-copilot-chip";

const chip = { id: "KAN-418", label: "KAN-418", ref: { kind: "task" as const, id: "task-418" } };

afterEach(cleanup);

describe("CoordinatorCopilotChipRow", () => {
  it("grows the remove button to the 44px touch-target minimum on a coarse pointer", () => {
    render(<CoordinatorCopilotChipRow chip={chip} onRemove={vi.fn()} />);
    const removeButton = screen.getByRole("button", { name: "Remove" });
    expect(removeButton.className).toContain("[@media(pointer:coarse)]:min-h-11");
    expect(removeButton.className).toContain("[@media(pointer:coarse)]:min-w-11");
  });
});

describe("CoordinatorCopilotEmptyIntro", () => {
  it("grows the suggestion button to the 44px touch-target minimum on a coarse pointer", () => {
    render(<CoordinatorCopilotEmptyIntro onSuggest={vi.fn()} />);
    const suggestionButton = screen.getByRole("button", {
      name: "What needs me first, and why?",
    });
    expect(suggestionButton.className).toContain("[@media(pointer:coarse)]:min-h-11");
  });
});
