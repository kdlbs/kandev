import { act, cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { useEditorResolverStore } from "@/lib/state/editor-resolver-store";
import { AppStatusBar } from "./app-status-bar";
import { APP_STATUS_LSP_ID } from "./app-status-bar-order";

const ACTIVE_KOTLIN_PATH = "src/Main.kt";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isFinePointer: true,
    isMobile: false,
  }),
}));

vi.mock("@/hooks/use-lsp", () => ({
  useLspStatus: () => ({
    status: { state: "disabled" },
    progress: {
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    },
    toggle: vi.fn(),
  }),
}));

afterEach(() => {
  cleanup();
  useDockviewStore.setState({
    activeFilePath: null,
    activeFileRepo: null,
    activePanelComponent: null,
    openFiles: new Map(),
  });
  useEditorResolverStore.setState((state) => ({
    providers: { ...state.providers, "code-editor": "monaco" },
  }));
});

function activateFile(
  path: string,
  {
    repo = "app",
    component = "file-editor",
    loaded = true,
    classified = true,
    isBinary = false,
  }: {
    repo?: string;
    component?: string;
    loaded?: boolean;
    classified?: boolean;
    isBinary?: boolean;
  } = {},
) {
  const file: FileEditorState = {
    path,
    repo,
    name: path.split("/").pop() ?? path,
    content: "fun main() = Unit",
    originalContent: "fun main() = Unit",
    originalHash: "hash",
    isDirty: false,
    ...(classified ? { isBinary } : {}),
  };
  useDockviewStore.setState({
    activeFilePath: path,
    activeFileRepo: repo,
    activePanelComponent: component,
    openFiles: loaded ? new Map([[buildRepoScopedItemId(path, repo), file]]) : new Map(),
  });
}

function renderBar(location: "toolbar" | "status_bar") {
  return render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultSettingsState.userSettings,
          appStatusBarEnabled: true,
          lspStatusLocation: location,
        },
      }}
    >
      <TooltipProvider>
        <AppStatusBar
          pathname="/tasks/task-1"
          activeWorkspaceId="workspace-1"
          activeTaskId="task-1"
          activeSessionId="session-1"
          density="full"
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("active-editor LSP status item integration", () => {
  it("renders the supported active file only for status-bar placement", () => {
    activateFile(ACTIVE_KOTLIN_PATH);
    renderBar("status_bar");

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeTruthy();
    expect(screen.getByTestId("app-status-lsp").textContent).toContain("Kotlin");
  });

  it("hides for toolbar placement and when the active panel becomes unsupported", () => {
    activateFile(ACTIVE_KOTLIN_PATH);
    const rendered = renderBar("toolbar");
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();

    rendered.unmount();
    activateFile("README.md");
    renderBar("status_bar");
    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();
  });

  it("hides when CodeMirror owns the active supported file", () => {
    useEditorResolverStore.setState((state) => ({
      providers: { ...state.providers, "code-editor": "codemirror" },
    }));
    activateFile(ACTIVE_KOTLIN_PATH);

    renderBar("status_bar");

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();
  });

  it("tracks whether the active panel has mounted a Monaco text editor", () => {
    activateFile(ACTIVE_KOTLIN_PATH, { loaded: false });
    renderBar("status_bar");
    const lspItem = () => document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`);

    expect(lspItem()).toBeNull();

    act(() => activateFile(ACTIVE_KOTLIN_PATH));
    expect(lspItem()).toBeTruthy();

    act(() => activateFile(ACTIVE_KOTLIN_PATH, { isBinary: true }));
    expect(lspItem()).toBeNull();

    act(() => activateFile(ACTIVE_KOTLIN_PATH, { component: "diff-viewer" }));
    expect(lspItem()).toBeNull();
  });

  it("hides while a restored file is waiting for text-or-binary classification", () => {
    activateFile(ACTIVE_KOTLIN_PATH, { classified: false });

    renderBar("status_bar");

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeNull();
  });
});
