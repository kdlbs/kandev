import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

vi.mock("@kandev/ui/command", () => ({
  Command: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandDialog: ({ children, open }: { children: ReactNode; open: boolean }) =>
    open ? <div>{children}</div> : null,
  CommandEmpty: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandGroup: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandInput: ({
    placeholder,
    value,
    onKeyDown,
  }: {
    placeholder: string;
    value: string;
    onKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => void;
  }) => (
    <input role="combobox" placeholder={placeholder} value={value} onKeyDown={onKeyDown} readOnly />
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
    selector({ userSettings: { keyboardShortcuts: {} } }),
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
    expect(screen.getByRole("tab", { name: "Commands" }).getAttribute("aria-selected")).toBe(
      "false",
    );
    expect(screen.getByRole("tab", { name: "Files" }).getAttribute("aria-selected")).toBe("false");
    expect(screen.getByRole("tab", { name: "Contents" }).getAttribute("aria-selected")).toBe(
      "true",
    );

    fireEvent.click(screen.getByRole("tab", { name: "Files" }));

    expect(onScopeChange).toHaveBeenCalledWith("search-files");
    expect(props.setSearch).not.toHaveBeenCalled();
    expect((screen.getByRole("combobox") as HTMLInputElement).value).toBe("needle");
  });

  it("cycles palette scopes with Tab and Shift+Tab", () => {
    const onScopeChange = vi.fn();
    const props = viewProps({ onScopeChange });
    render(<CommandPanelView {...props} />);
    const input = screen.getByRole("combobox");

    fireEvent.keyDown(input, { key: "Tab" });
    expect(onScopeChange).toHaveBeenLastCalledWith("commands");

    fireEvent.keyDown(input, { key: "Tab", shiftKey: true });
    expect(onScopeChange).toHaveBeenLastCalledWith("search-files");
  });
});
