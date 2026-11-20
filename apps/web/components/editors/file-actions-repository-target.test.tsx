import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, type TaskSession } from "@/lib/types/http";

const boundary = vi.hoisted(() => ({ open: vi.fn(), folder: vi.fn(), copy: vi.fn() }));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  openSessionInEditor: boundary.open,
  openSessionFolder: boundary.folder,
}));
vi.mock("@/lib/utils/copy-to-clipboard", () => ({ copyToClipboard: boundary.copy }));
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  ExternalVcsFileMenuItem: () => null,
  useExternalVcsFileStatus: () => null,
}));
vi.mock("@/components/editors/lsp-status-button", () => ({ LspStatusButton: () => null }));

import { MonacoEditorToolbar } from "./monaco/monaco-editor-toolbar";
import { CodeMirrorToolbar } from "./codemirror/codemirror-editor-toolbar";
import { EditorToolbarOverflowActions } from "./editor-toolbar-overflow";

const sid = sessionId("editor-session");
const filePath = "src/shared.ts";
const disabledAttribute = "data-disabled";
const session = {
  id: sid,
  task_id: taskId("editor-task"),
  state: "WAITING_FOR_INPUT",
  repository_id: repositoryId("repo-a"),
  worktree_id: "wt-a",
  worktree_path: "/task/a",
  workspace_path: "/task",
  worktrees: [
    {
      id: "assoc-a",
      session_id: sid,
      worktree_id: "wt-a",
      repository_id: repositoryId("repo-a"),
      worktree_path: "/task/a",
      position: 0,
    },
    {
      id: "assoc-b",
      session_id: sid,
      worktree_id: "wt-b",
      repository_id: repositoryId("repo-b"),
      worktree_path: "/task/b",
      position: 1,
    },
  ],
} as TaskSession;
const props = {
  path: filePath,
  repositoryName: "b",
  sessionId: sid,
  isDirty: false,
  isSaving: false,
  diffStats: null,
  wrapEnabled: false,
  enableComments: false,
  commentCount: 0,
  onToggleWrap: vi.fn(),
  onSave: vi.fn(),
};
const originalRect = HTMLElement.prototype.getBoundingClientRect;
const originalObserver = globalThis.ResizeObserver;

function editorInitialState(withSession: boolean, legacy: boolean) {
  return {
    ...defaultState,
    taskSessions: {
      ...defaultState.taskSessions,
      items: withSession ? { [sid]: legacy ? { ...session, worktrees: undefined } : session } : {},
    },
    worktrees: legacy
      ? { items: { "wt-a": { id: "wt-a", sessionId: sid, path: "/task/a" } } }
      : defaultState.worktrees,
    editors: {
      ...defaultState.editors,
      loaded: true,
      folderOpeningAvailable: true,
      items: [
        {
          id: "vscode",
          name: "VS Code",
          type: "vscode",
          kind: "built_in",
          enabled: true,
          installed: true,
        },
        {
          id: "embedded",
          name: "Embedded",
          type: "vscode",
          kind: "internal_vscode",
          enabled: true,
          installed: true,
        },
      ],
    },
    userSettings: { ...defaultState.userSettings, loaded: true },
  };
}

function mount(
  editor: "monaco" | "codemirror" | "overflow",
  narrow: boolean,
  name = "b",
  withSession = true,
  legacy = false,
) {
  class Observer {
    constructor(private callback: ResizeObserverCallback) {}
    observe(target: Element) {
      this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver);
    }
    disconnect() {}
    unobserve() {}
  }
  globalThis.ResizeObserver = Observer as unknown as typeof ResizeObserver;
  HTMLElement.prototype.getBoundingClientRect = function () {
    if (this.getAttribute("data-panel-header") === "true") {
      return {
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        right: narrow ? 480 : 1200,
        bottom: 30,
        width: narrow ? 480 : 1200,
        height: 30,
        toJSON: () => ({}),
      } as DOMRect;
    }
    return originalRect.call(this);
  };
  const initialState = editorInitialState(withSession, legacy);
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          {editor === "monaco" && (
            <MonacoEditorToolbar
              {...props}
              repositoryName={name}
              showDiffIndicators={false}
              lspStatus={{ state: "disabled" }}
              lspProgress={{
                initializingSince: null,
                active: [],
                completed: null,
                hasReportedProgress: false,
              }}
              lspLanguage={null}
              onToggleLsp={vi.fn()}
              onToggleDiffIndicators={vi.fn()}
            />
          )}
          {editor === "codemirror" && (
            <CodeMirrorToolbar {...props} repositoryName={name} repositoryId="repo-a" />
          )}
          {editor === "overflow" && (
            <EditorToolbarOverflowActions
              filePath={props.path}
              sessionId={sid}
              repositoryId="repo-a"
              repositoryName={name}
              isDirty={false}
              wrapEnabled={false}
              onToggleWrap={vi.fn()}
              previewKind="none"
            />
          )}
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

async function openMenu(narrow: boolean, container: HTMLElement) {
  const trigger = narrow
    ? await screen.findByRole("button", { name: "Show more actions" })
    : container.querySelector(".tabler-icon-external-link")?.closest("button");
  if (!trigger) throw new Error("Editor actions trigger not found");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  await screen.findByRole("menuitem", { name: /VS Code/ });
}

afterEach(() => {
  cleanup();
  HTMLElement.prototype.getBoundingClientRect = originalRect;
  globalThis.ResizeObserver = originalObserver;
  vi.clearAllMocks();
});

describe("editor file target routing", () => {
  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.2
  it.each([
    ["monaco", false],
    ["monaco", true],
    ["codemirror", true],
  ] as const)("routes %s overflow=%s to the named worktree", async (editor, narrow) => {
    boundary.open.mockResolvedValue({});
    const view = mount(editor, narrow);
    await openMenu(narrow, view.container);
    fireEvent.click(screen.getByRole("menuitem", { name: /VS Code/ }));
    await waitFor(() => expect(boundary.open).toHaveBeenCalledOnce());
    expect(boundary.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: "wt-b",
    });
  });

  it("shared editor overflow prefers the named file scope to the session primary ID", async () => {
    boundary.open.mockResolvedValue({});
    const view = mount("overflow", true);
    await openMenu(true, view.container);
    const external = screen.getByRole("menuitem", { name: /VS Code/ });
    expect(external.hasAttribute(disabledAttribute)).toBe(false);
    fireEvent.click(external);
    await waitFor(() => expect(boundary.open).toHaveBeenCalledOnce());
    expect(boundary.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: "wt-b",
    });
  });

  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.4
  it("legacy single worktree retains the omitted-ID default", async () => {
    boundary.open.mockResolvedValue({});
    const view = mount("codemirror", true, "", true, true);
    await openMenu(true, view.container);
    const external = screen.getByRole("menuitem", { name: /VS Code/ });
    expect(external.hasAttribute(disabledAttribute)).toBe(false);
    fireEvent.click(external);
    await waitFor(() => expect(boundary.open).toHaveBeenCalledOnce());
    expect(boundary.open.mock.calls[0][1]).toMatchObject({
      file_path: filePath,
      worktree_id: undefined,
    });
  });

  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.3, AC-UI-EXTERNAL-FILE-TARGET-001.5
  it("disables an unknown external target without disabling other actions", async () => {
    boundary.open.mockResolvedValue({});
    boundary.folder.mockResolvedValue({});
    boundary.copy.mockResolvedValue(true);
    const view = mount("codemirror", true, "unknown");
    await openMenu(true, view.container);
    const external = screen.getByRole("menuitem", { name: /VS Code/ });
    expect(external.getAttribute(disabledAttribute)).not.toBeNull();
    fireEvent.click(external);
    expect(boundary.open).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("menuitem", { name: /Embedded/ }));
    await waitFor(() => expect(boundary.open).toHaveBeenCalledOnce());
    expect(boundary.open.mock.calls[0][1]).toMatchObject({
      editor_id: "embedded",
      file_path: filePath,
      worktree_id: undefined,
    });
    await openMenu(true, view.container);
    fireEvent.click(screen.getByRole("menuitem", { name: /Copy path/ }));
    await waitFor(() => expect(boundary.copy).toHaveBeenCalledOnce());
    await openMenu(true, view.container);
    fireEvent.click(screen.getByRole("menuitem", { name: /Open folder/ }));
    await waitFor(() => expect(boundary.folder).toHaveBeenCalledOnce());
  });
  it("does not launch an external file without session metadata", async () => {
    const view = mount("codemirror", true, "b", false);
    await openMenu(true, view.container);
    const external = screen.getByRole("menuitem", { name: /VS Code/ });
    expect(external.getAttribute(disabledAttribute)).not.toBeNull();
    fireEvent.click(external);
    expect(boundary.open).not.toHaveBeenCalled();
  });
});
