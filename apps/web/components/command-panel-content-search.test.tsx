import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

vi.mock("@kandev/ui/command", () => ({
  Command: ({ children, shouldFilter }: { children: ReactNode; shouldFilter?: boolean }) => (
    <div data-testid="command-root" data-should-filter={String(shouldFilter)}>
      {children}
    </div>
  ),
  CommandDialog: ({ children, open }: { children: ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
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
    <section data-testid={testId} data-repository={repository}>
      {heading && <h2>{heading}</h2>}
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
  CommandItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
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
  MODE_SEARCH_CONTENT,
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

  it("makes all palette scopes visible and switches without clearing the query", () => {
    const onScopeChange = vi.fn();
    const props = viewProps({ onScopeChange });
    render(<CommandPanelView {...props} />);

    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(3);
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

  it("hides workspace modes and leaves Tab alone outside a task workbench", () => {
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

    expect(screen.queryByRole("tablist", { name: "Command palette mode" })).toBeNull();
    expect(screen.queryByText("Switch mode")).toBeNull();
    expect(fireEvent.keyDown(input, { key: "Tab" })).toBe(true);
    expect(onScopeChange).not.toHaveBeenCalled();
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
            { id: "matching-command", label: "Needle command", group: "Actions" },
            { id: "unrelated-command", label: "Unrelated action", group: "Actions" },
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
