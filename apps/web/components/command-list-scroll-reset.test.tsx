import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createRef, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { Command, CommandInput, CommandItem, CommandList } from "@kandev/ui/command";

const LIST_TEST_ID = "command-list";
const SEARCH_PLACEHOLDER = "Search commands";

function getList() {
  return screen.getByTestId(LIST_TEST_ID) as HTMLDivElement;
}

function renderItems() {
  return ["Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta"].map((label) => (
    <CommandItem key={label} value={label.toLowerCase()}>
      {label}
    </CommandItem>
  ));
}

function ControlledCommandHarness() {
  const [search, setSearch] = useState("");

  return (
    <Command shouldFilter={false}>
      <CommandInput placeholder={SEARCH_PLACEHOLDER} value={search} onValueChange={setSearch} />
      <CommandList data-testid={LIST_TEST_ID}>{renderItems()}</CommandList>
    </Command>
  );
}

afterEach(() => {
  cleanup();
});

describe("CommandList scroll reset", () => {
  it("resets scrollTop when the cmdk query changes", () => {
    render(
      <Command>
        <CommandInput placeholder={SEARCH_PLACEHOLDER} />
        <CommandList data-testid={LIST_TEST_ID}>{renderItems()}</CommandList>
      </Command>,
    );

    const list = getList();
    list.scrollTop = 123;

    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value: "be" },
    });

    expect(list.scrollTop).toBe(0);
  });

  it("resets scrollTop for controlled inputs that disable cmdk filtering", () => {
    render(<ControlledCommandHarness />);

    const list = getList();
    list.scrollTop = 123;

    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value: "ga" },
    });

    expect(list.scrollTop).toBe(0);
  });

  it("resets scrollTop when the query is cleared", () => {
    render(
      <Command>
        <CommandInput placeholder={SEARCH_PLACEHOLDER} />
        <CommandList data-testid={LIST_TEST_ID}>{renderItems()}</CommandList>
      </Command>,
    );

    const input = screen.getByPlaceholderText(SEARCH_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "de" } });

    const list = getList();
    list.scrollTop = 88;

    fireEvent.change(input, { target: { value: "" } });

    expect(list.scrollTop).toBe(0);
  });

  it("forwards external refs to the underlying list element", () => {
    const ref = createRef<HTMLDivElement>();

    render(
      <Command>
        <CommandInput placeholder={SEARCH_PLACEHOLDER} />
        <CommandList ref={ref} data-testid={LIST_TEST_ID}>
          {renderItems()}
        </CommandList>
      </Command>,
    );

    expect(ref.current).toBe(getList());
  });
});
