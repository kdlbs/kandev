import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationStartTaskMenu } from "./integration-start-task-menu";
import type { IntegrationPresetIconName } from "./integration-preset-icons";

afterEach(cleanup);

function TestIcon({ className }: { className?: string }) {
  return <span className={className}>icon</span>;
}

describe("IntegrationStartTaskMenu", () => {
  it.each([
    ["eye", "tabler-icon-eye"],
    ["message", "tabler-icon-message-dots"],
    ["tool", "tabler-icon-tool"],
  ])("maps the semantic %s icon to the native Tabler glyph", async (iconName, className) => {
    render(
      <IntegrationStartTaskMenu
        presets={[
          {
            id: iconName,
            label: iconName,
            hint: `${iconName} action`,
            iconName: iconName as IntegrationPresetIconName,
          },
        ]}
        onSelect={vi.fn()}
        itemTestId="semantic-preset"
      />,
    );

    fireEvent.pointerDown(screen.getByRole("button", { name: "Task" }), {
      button: 0,
      ctrlKey: false,
    });
    const item = await screen.findByTestId("semantic-preset");
    expect(item.querySelector(`.${className}`)).not.toBeNull();
  });

  it("uses touch-sized controls and returns the selected preset", async () => {
    const onSelect = vi.fn();
    render(
      <IntegrationStartTaskMenu
        presets={[{ id: "review", label: "Review", hint: "Review this change", icon: TestIcon }]}
        onSelect={onSelect}
        triggerTestId="change-task-trigger"
        itemTestId="change-task-preset"
      />,
    );

    const trigger = screen.getByTestId("change-task-trigger");
    expect(trigger.className).toContain("h-11");
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    const item = await screen.findByTestId("change-task-preset");
    expect(item.className).toContain("min-h-11");
    fireEvent.click(item);
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "review" }));
  });

  it("renders nothing without presets", () => {
    const { container } = render(<IntegrationStartTaskMenu presets={[]} onSelect={vi.fn()} />);
    expect(container.childElementCount).toBe(0);
  });
});
