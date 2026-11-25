import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlaceholderList, PromptSection } from "./prompt-section";

const promptEditor = vi.hoisted(() => vi.fn());

vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div data-testid="automation-prompt-editor" />;
  },
}));

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("PromptSection", () => {
  it("uses shared prompt references and automation placeholders", () => {
    render(
      <PromptSection
        value="Run {{pr.title}}"
        isDirty
        onChange={vi.fn()}
        placeholders={[{ key: "pr.title", description: "PR title", example: "Ship it" }]}
      />,
    );

    expect(promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        language: "plaintext",
        isDirty: true,
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "pr.title" })]),
      }),
    );
  });

  it("leaves the placeholder list to the shared disclosure", () => {
    render(
      <PromptSection
        value=""
        isDirty={false}
        onChange={vi.fn()}
        placeholders={[{ key: "pr.title", description: "PR title", example: "Ship it" }]}
      />,
    );
    expect(screen.queryByText("{{pr.title}}")).toBeNull();
  });
});

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.4
describe("PlaceholderList", () => {
  const placeholders = [
    { key: "pr.title", description: "PR title", example: "Ship it" },
    { key: "pr.number", description: "PR number", example: "7" },
  ];

  it("lists the placeholders once, behind a collapsed disclosure", () => {
    render(<PlaceholderList placeholders={placeholders} />);
    const toggle = screen.getByRole("button", { name: "Show placeholders" });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("{{pr.title}}")).toBeNull();
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("{{pr.title}}")).toBeTruthy();
    expect(screen.getByText("{{pr.number}}")).toBeTruthy();
  });

  it("renders nothing when the trigger offers no placeholders", () => {
    const { container } = render(<PlaceholderList placeholders={[]} />);
    expect(container.textContent).toBe("");
  });
});
