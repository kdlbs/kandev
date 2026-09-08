import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

vi.mock("@kandev/ui/command", () => ({
  Command: ({ children, shouldFilter }: { children: ReactNode; shouldFilter?: boolean }) => (
    <div data-testid="command-root" data-should-filter={String(shouldFilter)}>
      {children}
    </div>
  ),
  CommandDialog: ({
    children,
    open,
    onOpenChange,
  }: {
    children: ReactNode;
    open: boolean;
    onOpenChange: (open: boolean) => void;
  }) =>
    open ? (
      <div>
        <button type="button" onClick={() => onOpenChange(false)}>
          Dismiss test dialog
        </button>
        {children}
      </div>
    ) : null,
  CommandEmpty: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandGroup: ({
    children,
    heading,
    "data-testid": testId,
    "data-repository": repository,
  }: {
    children: ReactNode;
    heading?: ReactNode;
    "data-testid"?: string;
    "data-repository"?: string;
  }) => (
    <section {...{ "cmdk-group": "" }} data-testid={testId} data-repository={repository}>
      {heading && (
        <div {...{ "cmdk-group-heading": "" }} aria-hidden>
          {heading}
        </div>
      )}
      {children}
    </section>
  ),
  CommandInput: ({
    placeholder,
    value,
    onKeyDown,
  }: {
    placeholder: string;
    value: string;
    onKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => void;
  }) => (
    <div data-slot="command-input-wrapper">
      <input
        role="combobox"
        placeholder={placeholder}
        value={value}
        onKeyDown={onKeyDown}
        readOnly
      />
    </div>
  ),
  // Forwards onSelect: without it the mock silently swallows every selection
  // handler and any assertion about picking a row would prove nothing.
  CommandItem: ({ children, onSelect }: { children: ReactNode; onSelect?: () => void }) => (
    <div role="option" aria-selected="false" onClick={onSelect}>
      {children}
    </div>
  ),
  CommandList: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandShortcut: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
  KbdGroup: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@kandev/ui/badge", () => ({
  Badge: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { keyboardShortcuts: {} },
      taskSessions: { items: {} },
      kanban: { tasks: [] },
      repositories: { itemsByWorkspaceId: {} },
    }),
}));

type MockContentSearchProps = {
  onSelect: (result: WorkspaceContentSearchResult) => void;
  results: WorkspaceContentSearchResult[];
};

const mockContentSearch = vi.fn(({ onSelect, results }: MockContentSearchProps) => (
  <button data-testid="mock-content-search" onClick={() => onSelect(results[0])}>
    Content results
  </button>
));

vi.mock("./workspace-content-search", () => ({
  WorkspaceContentSearch: (props: MockContentSearchProps) => mockContentSearch(props),
}));

import {
  CommandPanelView,
  MODE_COMMANDS,
  MODE_SEARCH_CONTENT,
  MODE_SEARCH_TASKS,
  type CommandPanelViewProps,
} from "./command-panel-footer";

const result: WorkspaceContentSearchResult = {
  repository_name: "web",
  path: "src/app.tsx",
  line: 5,
  column: 3,
  preview: "needle",
  match_ranges: [{ start: 0, end: 6 }],
};

const ARIA_SELECTED_ATTRIBUTE = "aria-selected";
const CMDK_GROUP_HEADING_SELECTOR = "[cmdk-group-heading]";
const ACTIONS_GROUP = "Actions";
const CONFIRMATION_LABEL = "Remove selected item";

function viewProps(overrides: Partial<CommandPanelViewProps> = {}): CommandPanelViewProps {
  return {
    open: true,
    setOpen: vi.fn(),
    mode: MODE_SEARCH_CONTENT,
    inputCommand: null,
    selectedValue: "",
    setSelectedValue: vi.fn(),
    search: "needle",
    setSearch: vi.fn(),
    handleKeyDown: vi.fn(),
    onScopeChange: vi.fn(),
    goBack: vi.fn(),
    fileResults: [],
    isSearchingFiles: false,
    handleFileSelect: vi.fn(),
    contentResults: [result],
    isSearchingContent: false,
    contentSearchError: null,
    activeSessionId: "session-1",
    workspaceSearchAvailable: true,
    handleContentSelect: vi.fn(),
    commands: [],
    grouped: [],
    handleSelect: vi.fn(),
    isSearching: false,
    taskResults: [],
    stepMap: new Map(),
    repoMap: new Map(),
    liveTasksById: new Map(),
    lastStepIdByWorkflowId: new Map(),
    handleTaskSelect: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  mockContentSearch.mockClear();
});

describe("CommandPanelView task content search mode", () => {
  it("renders a dedicated input and forwards repository results", () => {
    const props = viewProps();
    render(<CommandPanelView {...props} />);

    expect(screen.getByPlaceholderText("Search task contents…")).toBeTruthy();
    expect(screen.getByText("Contents")).toBeTruthy();
    expect(mockContentSearch).toHaveBeenCalledWith(
      expect.objectContaining({
        results: [result],
        isSearching: false,
        error: null,
        search: "needle",
        sessionId: "session-1",
      }),
    );

    fireEvent.click(screen.getByTestId("mock-content-search"));
    expect(props.handleContentSelect).toHaveBeenCalledWith(result);
  });
});

describe("CommandPanelView scope switcher", () => {
  it("makes all palette scopes visible and switches without clearing the query", () => {
    const onScopeChange = vi.fn();
    const props = viewProps({ onScopeChange });
    render(<CommandPanelView {...props} />);

    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((tab) => tab.getAttribute("aria-label"))).toEqual([
      "Commands",
      "Tasks",
      "Files",
      "Contents",
    ]);
    expect(
      screen.getByRole("tab", { name: "Commands" }).getAttribute(ARIA_SELECTED_ATTRIBUTE),
    ).toBe("false");
    expect(screen.getByRole("tab", { name: "Files" }).getAttribute(ARIA_SELECTED_ATTRIBUTE)).toBe(
      "false",
    );
    expect(
      screen.getByRole("tab", { name: "Contents" }).getAttribute(ARIA_SELECTED_ATTRIBUTE),
    ).toBe("true");

    fireEvent.click(screen.getByRole("tab", { name: "Files" }));

    expect(onScopeChange).toHaveBeenCalledWith("search-files");
    expect(props.setSearch).not.toHaveBeenCalled();
    expect((screen.getByRole("combobox") as HTMLInputElement).value).toBe("needle");
  });

  it("keeps the mode selector inline with the search input", () => {
    render(<CommandPanelView {...viewProps()} />);

    const inputWrapper = screen.getByRole("combobox").closest("[data-slot=command-input-wrapper]");
    const switcher = screen.getByRole("tablist", { name: "Command palette mode" });

    expect(switcher.parentElement).toBe(inputWrapper?.parentElement);
  });

  it("keeps balanced breathing room between the input and header divider", () => {
    render(<CommandPanelView {...viewProps()} />);

    const inputWrapper = screen.getByRole("combobox").closest("[data-slot=command-input-wrapper]");

    expect(inputWrapper?.parentElement?.className).toContain(
      "[&>[data-slot=command-input-wrapper]]:pb-1",
    );
  });

  it("uses a low-chrome text selector with an active underline", () => {
    render(<CommandPanelView {...viewProps()} />);

    const switcher = screen.getByRole("tablist", { name: "Command palette mode" });
    expect(switcher.querySelector("kbd")).toBeNull();
    expect(switcher.querySelectorAll("svg")).toHaveLength(0);
    expect(screen.queryByTestId("command-panel-scope-indicator")).toBeNull();
    for (const tab of screen.getAllByRole("tab")) {
      expect(tab.getAttribute("tabindex")).toBe("-1");
      expect(tab.className).toContain(
        tab.getAttribute(ARIA_SELECTED_ATTRIBUTE) === "true"
          ? "after:opacity-100"
          : "after:opacity-0",
      );
    }
  });

  it("cycles palette scopes with Tab and Shift+Tab", () => {
    const onScopeChange = vi.fn();
    const props = viewProps({ onScopeChange });
    render(<CommandPanelView {...props} />);
    const input = screen.getByRole("combobox");
    input.focus();

    expect(fireEvent.keyDown(input, { key: "Tab" })).toBe(false);
    expect(onScopeChange).toHaveBeenLastCalledWith("commands");
    expect(document.activeElement).toBe(input);

    expect(fireEvent.keyDown(input, { key: "Tab", shiftKey: true })).toBe(false);
    expect(onScopeChange).toHaveBeenLastCalledWith("search-files");
    expect(document.activeElement).toBe(input);
  });

  it("hides workspace modes but keeps commands and tasks outside a task workbench", () => {
    const onScopeChange = vi.fn();
    render(
      <CommandPanelView
        {...viewProps({
          mode: "commands",
          workspaceSearchAvailable: false,
          onScopeChange,
        })}
      />,
    );
    const input = screen.getByRole("combobox");

    expect(screen.getAllByRole("tab").map((tab) => tab.getAttribute("aria-label"))).toEqual([
      "Commands",
      "Tasks",
    ]);
    // Tab still cycles, but only between the two scopes that need no worktree.
    expect(fireEvent.keyDown(input, { key: "Tab" })).toBe(false);
    expect(onScopeChange).toHaveBeenLastCalledWith("search-tasks");
    expect(fireEvent.keyDown(input, { key: "Tab", shiftKey: true })).toBe(false);
    expect(onScopeChange).toHaveBeenLastCalledWith("search-tasks");
  });
});

describe("CommandPanelView search-only commands", () => {
  const FONT_SIZE_LABEL = "Terminal Font Size";
  const goToSettings = {
    id: "nav-settings",
    label: "Go to Settings",
    group: "Navigation",
    action: vi.fn(),
  };
  const fontSize = {
    id: "setting:terminal-font-size",
    label: FONT_SIZE_LABEL,
    group: "Settings",
    context: "Settings › Terminal & Editors",
    searchOnly: true,
    action: vi.fn(),
  };

  it("keeps granular settings hidden before typing", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "",
          commands: [goToSettings, fontSize],
          grouped: [
            ["Navigation", [goToSettings]],
            ["Settings", [fontSize]],
          ],
        })}
      />,
    );

    expect(screen.getByText("Go to Settings")).toBeTruthy();
    expect(screen.queryByText(FONT_SIZE_LABEL)).toBeNull();
  });

  it("shows a matching granular setting with owning context after typing", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "font size",
          commands: [goToSettings, fontSize],
          grouped: [],
        })}
      />,
    );

    expect(screen.getByText(FONT_SIZE_LABEL)).toBeTruthy();
    expect(screen.getByText("Settings › Terminal & Editors")).toBeTruthy();
    expect(screen.getByText("Settings", { selector: CMDK_GROUP_HEADING_SELECTOR })).toBeTruthy();
    expect(screen.queryByText("Commands", { selector: CMDK_GROUP_HEADING_SELECTOR })).toBeNull();
  });

  it("separates regular and granular matches into Commands and Settings", () => {
    const fontSizeGuide = {
      id: "help:terminal-font-size",
      label: "Terminal Font Size Guide",
      group: "Help",
      action: vi.fn(),
    };
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "terminal font size",
          commands: [fontSizeGuide, fontSize],
          grouped: [],
        })}
      />,
    );

    expect(
      screen
        .getByText("Terminal Font Size Guide")
        .closest("[cmdk-group]")
        ?.querySelector(CMDK_GROUP_HEADING_SELECTOR)?.textContent,
    ).toBe("Commands");
    expect(
      screen
        .getByText(FONT_SIZE_LABEL, { exact: true })
        .closest("[cmdk-group]")
        ?.querySelector(CMDK_GROUP_HEADING_SELECTOR)?.textContent,
    ).toBe("Settings");
  });
});

describe("CommandPanelView mode result safety", () => {
  it("does not delegate filtering to cmdk while palette mode groups swap", () => {
    render(<CommandPanelView {...viewProps({ mode: "commands" })} />);

    expect(screen.getByTestId("command-root").getAttribute("data-should-filter")).toBe("false");
  });

  it("filters command results before rendering them", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: "commands",
          search: "needle",
          commands: [
            { id: "matching-command", label: "Needle command", group: ACTIONS_GROUP },
            { id: "unrelated-command", label: "Unrelated action", group: ACTIONS_GROUP },
          ],
        })}
      />,
    );

    expect(screen.getByText("Needle command")).toBeTruthy();
    expect(screen.queryByText("Unrelated action")).toBeNull();
  });

  it("returns a stale workspace-search mode to commands when context disappears", () => {
    const onScopeChange = vi.fn();

    render(
      <CommandPanelView
        {...viewProps({
          mode: "search-content",
          workspaceSearchAvailable: false,
          onScopeChange,
        })}
      />,
    );

    expect(onScopeChange).toHaveBeenCalledWith("commands");
  });

  it("groups file matches by repository", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: "search-files",
          search: "shared",
          fileResults: [
            {
              repository_name: "backend",
              path: "backend/src/shared-search.go",
            },
            {
              repository_name: "frontend",
              path: "frontend/src/shared-search.ts",
            },
          ],
        })}
      />,
    );

    const groups = screen.getAllByTestId("file-search-repo-group");
    expect(groups).toHaveLength(2);
    expect(groups[0].getAttribute("data-repository")).toBe("backend");
    expect(groups[1].getAttribute("data-repository")).toBe("frontend");
  });
});

describe("CommandPanelView confirmations", () => {
  it("labels generic confirmations from their command and hides the command row", () => {
    const confirmationCommand = {
      id: "destructive-command",
      label: CONFIRMATION_LABEL,
      group: ACTIONS_GROUP,
      confirmation: <div>Confirm removal</div>,
    };
    const regularCommand = {
      id: "regular-command",
      label: "Keep working",
      group: ACTIONS_GROUP,
    };

    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "",
          commands: [confirmationCommand, regularCommand],
          grouped: [[ACTIONS_GROUP, [confirmationCommand, regularCommand]]],
        })}
      />,
    );

    expect(screen.getByRole("alertdialog", { name: CONFIRMATION_LABEL })).toBeTruthy();
    expect(screen.getByText("Confirm removal")).toBeTruthy();
    expect(screen.queryByText(CONFIRMATION_LABEL)).toBeNull();
    expect(screen.getByText("Keep working")).toBeTruthy();
  });

  it("dismisses an owned confirmation when the palette closes", () => {
    const onConfirmationDismiss = vi.fn();
    const setOpen = vi.fn();
    const confirmationCommand = {
      id: "destructive-command",
      label: CONFIRMATION_LABEL,
      group: ACTIONS_GROUP,
      confirmation: <div>Confirm removal</div>,
      onConfirmationDismiss,
    };

    render(
      <CommandPanelView
        {...viewProps({
          setOpen,
          mode: MODE_COMMANDS,
          commands: [confirmationCommand],
          grouped: [[ACTIONS_GROUP, [confirmationCommand]]],
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Dismiss test dialog" }));

    expect(onConfirmationDismiss).toHaveBeenCalledOnce();
    expect(setOpen).toHaveBeenCalledWith(false);
  });
});

function task(id: string, title: string): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    created_at: "2026-08-24T09:00:00Z",
    updated_at: "2026-08-24T09:00:00Z",
  };
}

describe("CommandPanelView commands scope ordering", () => {
  const archiveCommand = {
    id: "task-archive",
    label: "Archive task",
    group: "Task",
    action: vi.fn(),
  };

  function groupHeadings() {
    return Array.from(document.querySelectorAll(CMDK_GROUP_HEADING_SELECTOR)).map(
      (heading) => heading.textContent,
    );
  }

  it("puts matching commands above the task preview once the user types", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "archive",
          commands: [archiveCommand],
          grouped: [],
          taskResults: [task("task-1", "Close every remaining archive item")],
        })}
      />,
    );

    expect(groupHeadings()).toEqual(["Commands", "Tasks"]);
  });

  it("keeps the active task list on top while nothing is typed", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_COMMANDS,
          search: "",
          commands: [archiveCommand],
          grouped: [["Task", [archiveCommand]]],
          taskResults: [task("task-1", "Resume yesterday's work")],
        })}
      />,
    );

    expect(groupHeadings()).toEqual(["Active Tasks", "Task"]);
  });
});

describe("CommandPanelView tasks scope", () => {
  it("lists task results and opens the one that is picked", () => {
    const target = task("task-1", "Palette task");
    const props = viewProps({
      mode: MODE_SEARCH_TASKS,
      search: "palette",
      taskResults: [target],
    });
    render(<CommandPanelView {...props} />);

    expect(screen.getByPlaceholderText("Search for tasks...")).toBeTruthy();
    expect(screen.getByText("Palette task")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Tasks" }).getAttribute(ARIA_SELECTED_ATTRIBUTE)).toBe(
      "true",
    );

    fireEvent.click(screen.getByText("Palette task"));
    expect(props.handleTaskSelect).toHaveBeenCalledWith(target);
  });

  it("reports an empty task search instead of falling back to commands", () => {
    render(
      <CommandPanelView
        {...viewProps({
          mode: MODE_SEARCH_TASKS,
          search: "nothing",
          taskResults: [],
          commands: [{ id: "nav-home", label: "Go Home", group: "Navigation" }],
        })}
      />,
    );

    expect(screen.getByText("No tasks found.")).toBeTruthy();
    expect(screen.queryByText("Go Home")).toBeNull();
  });
});
