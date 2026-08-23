import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Combobox, type ComboboxOption } from "@/components/combobox";

afterEach(() => cleanup());

const options: ComboboxOption[] = [
  { value: "first", label: "First" },
  { value: "current", label: "Current" },
  { value: "last", label: "Last" },
];
const selectedAttribute = "data-selected";

describe("Combobox", () => {
  it("puts the current option first with a persistent selected surface", () => {
    render(
      <Combobox
        options={options}
        value="current"
        onValueChange={vi.fn()}
        ariaLabel="Choose an agent"
        dropdownLabel="Agents"
      />,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Choose an agent" }));

    const renderedOptions = screen.getAllByRole("option");
    expect(renderedOptions.map((option) => option.textContent?.trim())).toEqual([
      "Current",
      "First",
      "Last",
    ]);
    expect(renderedOptions[0].className).toContain("bg-card");
    expect(renderedOptions[0].className).toContain("border-primary/50");

    const searchInput = document.querySelector("[cmdk-input]");
    expect(searchInput).not.toBeNull();
    expect(renderedOptions[0].getAttribute(selectedAttribute)).toBe("true");

    fireEvent.keyDown(searchInput!, { key: "ArrowDown" });
    expect(renderedOptions[0].getAttribute(selectedAttribute)).toBe("false");
    expect(renderedOptions[1].getAttribute(selectedAttribute)).toBe("true");

    fireEvent.keyDown(searchInput!, { key: "ArrowUp" });
    expect(renderedOptions[0].getAttribute(selectedAttribute)).toBe("true");
  });

  it("keeps a disabled current option first and non-selectable", () => {
    render(
      <TooltipProvider>
        <Combobox
          options={[
            { value: "available", label: "Available" },
            {
              value: "current",
              label: "Unavailable current",
              disabled: true,
              disabledReason: "No longer available",
            },
          ]}
          value="current"
          onValueChange={vi.fn()}
          ariaLabel="Choose an agent"
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Choose an agent" }));

    const renderedOptions = screen.getAllByRole("option");
    expect(renderedOptions.map((option) => option.textContent?.trim())).toEqual([
      "Unavailable current",
      "Available",
    ]);
    expect(renderedOptions[0].getAttribute("aria-disabled")).toBe("true");
    expect(renderedOptions[0].className).toContain("bg-card");
  });
});
