import { useEffect } from "react";
import { StateProvider } from "./state-provider";
import { CommandRegistryProvider, useCommandPanelOpen } from "@/lib/commands/command-registry";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { CommandPanel } from "./command-panel";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CommandPanelView, type CommandPanelViewProps } from "./command-panel-footer";
import { findFirstMatchingCommand, selectCommandSearchResult } from "@/lib/commands/search";
import type { CommandItem } from "@/lib/commands/types";

afterEach(cleanup);

const disabledCommand = { id: "duplicate", label: "Duplicate", group: "Tasks", disabled: true };

it("never chooses a disabled command as the actionable result", () => {
  const commands: CommandItem[] = [disabledCommand];
  expect(findFirstMatchingCommand(commands, "Duplicate")).toBeUndefined();
  expect(
    selectCommandSearchResult({
      commands,
      search: "Duplicate",
      preferredValue: "duplicate",
      taskResultValues: [],
      commandsLeadResults: true,
    }),
  ).toBe("");
});

it("renders disabled commands without allowing selection", () => {
  const handleSelect = vi.fn();
  const props: CommandPanelViewProps = {
    open: true,
    setOpen: vi.fn(),
    mode: "commands",
    inputCommand: null,
    selectedValue: "",
    setSelectedValue: vi.fn(),
    search: "Duplicate",
    setSearch: vi.fn(),
    handleKeyDown: vi.fn(),
    onScopeChange: vi.fn(),
    goBack: vi.fn(),
    fileResults: [],
    isSearchingFiles: false,
    handleFileSelect: vi.fn(),
    contentResults: [],
    isSearchingContent: false,
    contentSearchError: null,
    activeSessionId: null,
    workspaceSearchAvailable: false,
    handleContentSelect: vi.fn(),
    commands: [disabledCommand],
    grouped: [["Tasks", [disabledCommand]]],
    handleSelect,
    isSearching: false,
    taskResults: [],
    stepMap: new Map(),
    repoMap: new Map(),
    liveTasksById: new Map(),
    lastStepIdByWorkflowId: new Map(),
    handleTaskSelect: vi.fn(),
  };
  render(
    <StateProvider>
      <CommandPanelView {...props} />
    </StateProvider>,
  );
  const option = screen.getByRole("option", { name: "Duplicate" });
  fireEvent.click(option);
  expect(handleSelect).not.toHaveBeenCalled();
  expect(option.getAttribute("aria-disabled")).toBe("true");
});

const review = vi.fn();
const commands = [
  {
    id: "move",
    label: "Move to",
    group: "Tasks",
    children: [{ id: "review", label: "Review", group: "Steps", action: review }],
  },
];
function Register({ items = commands }: { items?: CommandItem[] }) {
  useRegisterCommands(items);
  const { setOpen } = useCommandPanelOpen();
  useEffect(() => {
    setOpen(true);
  }, [setOpen]);
  return <CommandPanel />;
}
it("navigates into current child commands and back without executing the parent", async () => {
  render(
    <StateProvider>
      <CommandRegistryProvider>
        <Register />
      </CommandRegistryProvider>
    </StateProvider>,
  );
  fireEvent.click(await screen.findByRole("option", { name: "Move to" }));
  expect(await screen.findByRole("option", { name: "Review" })).toBeTruthy();
  expect(review).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
  expect(await screen.findByRole("option", { name: "Move to" })).toBeTruthy();
});

it("drops stale nested choices when their registered parent disappears", async () => {
  const view = render(
    <StateProvider>
      <CommandRegistryProvider>
        <Register />
      </CommandRegistryProvider>
    </StateProvider>,
  );
  fireEvent.click(await screen.findByRole("option", { name: "Move to" }));
  expect(await screen.findByRole("option", { name: "Review" })).toBeTruthy();
  const replacement = [{ id: "pin", label: "Pin", group: "Tasks", action: vi.fn() }];
  view.rerender(
    <StateProvider>
      <CommandRegistryProvider>
        <Register items={replacement} />
      </CommandRegistryProvider>
    </StateProvider>,
  );
  expect(await screen.findByRole("option", { name: "Pin" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "Review" })).toBeNull();
});

it.each(["ctrlKey", "metaKey"])(
  "runs a destination's immediate action with %s+Enter",
  async (modifier) => {
    const openOptions = vi.fn();
    const moveNow = vi.fn();
    const items: CommandItem[] = [
      {
        id: "destination",
        label: "Verify",
        group: "Steps",
        action: openOptions,
        immediateAction: moveNow,
      },
    ];
    render(
      <StateProvider>
        <CommandRegistryProvider>
          <Register items={items} />
        </CommandRegistryProvider>
      </StateProvider>,
    );
    await screen.findByRole("option", { name: /Verify/ });
    fireEvent.keyDown(screen.getByRole("combobox"), { key: "Enter", [modifier]: true });
    expect(moveNow).toHaveBeenCalledOnce();
    expect(openOptions).not.toHaveBeenCalled();
  },
);
