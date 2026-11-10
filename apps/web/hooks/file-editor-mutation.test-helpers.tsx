import { StrictMode, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { useFileEditors } from "./use-file-editors";

const hoistedMocks = vi.hoisted(() => ({
  client: {},
  save: vi.fn(),
  delete: vi.fn(),
  toast: vi.fn(),
  lsp: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => hoistedMocks.client }));
vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => hoistedMocks.save(...args),
  deleteFile: (...args: unknown[]) => hoistedMocks.delete(...args),
  requestFileContent: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: hoistedMocks.toast }) }));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: { saveDocument: (...args: unknown[]) => hoistedMocks.lsp(...args) },
}));
vi.mock("@/components/editors/monaco/monaco-init", () => ({ getMonacoInstance: () => null }));

export const mocks = hoistedMocks;

export const PATH = "src/foo.ts";
export const REPO = "frontend";
export type Reply = { success: boolean; new_hash?: string; error?: string; resolution?: string };
type Panel = {
  id: string;
  params: Record<string, unknown>;
  api: { updateParameters: ReturnType<typeof vi.fn> };
  setTitle: ReturnType<typeof vi.fn>;
};
let visit = 0;

export function defer<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function makePanel(repo = REPO, preview = false) {
  const key = buildRepoScopedItemId(PATH, repo);
  const panel: Panel = {
    id: preview ? PREVIEW_FILE_EDITOR_ID : `file:${key}`,
    params: { path: PATH, repo, isDirty: true, ...(preview ? { previewItemId: key } : {}) },
    api: {
      updateParameters: vi.fn((params: Record<string, unknown>) => {
        panel.params = params;
      }),
    },
    setTitle: vi.fn(),
  };
  return panel;
}

export function panelHost() {
  const panels = new Map<string, Panel>();
  const removed = new Set<(panel: Panel) => void>();
  const api = {
    getPanel: (id: string) => panels.get(id),
    onDidRemovePanel: (listener: (panel: Panel) => void) => {
      removed.add(listener);
      return { dispose: () => removed.delete(listener) };
    },
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
    removePanel: vi.fn((panel: Panel) => {
      panels.delete(panel.id);
      removed.forEach((listener) => listener(panel));
    }),
  };
  return { panels, api, dockApi: api as unknown as DockviewApi };
}

function wrapper({ children }: { children: ReactNode }) {
  return (
    <StrictMode>
      <StateProvider>{children}</StateProvider>
    </StrictMode>
  );
}

export function setup() {
  const host = panelHost();
  useDockviewStore.setState({ api: host.dockApi });
  const hook = renderHook(() => ({ actions: useFileEditors(), store: useAppStoreApi() }), {
    wrapper,
  });
  const sessionA = `editor-A-${++visit}`;
  const sessionB = `editor-B-${visit}`;
  const navigate = (session: string | null) => {
    act(() =>
      hook.result.current.store.setState((state) => ({
        tasks: { ...state.tasks, activeSessionId: session },
      })),
    );
  };
  navigate(sessionA);
  const seed = (
    label = "A",
    repo = REPO,
    preview = false,
    extra: Partial<FileEditorState> = {},
  ) => {
    const panel = makePanel(repo, preview);
    host.panels.set(panel.id, panel);
    act(() =>
      useDockviewStore.getState().setFileState(buildRepoScopedItemId(PATH, repo), {
        path: PATH,
        repo,
        name: "foo.ts",
        content: `${label} buffer`,
        originalContent: `${label} disk`,
        originalHash: `hash-${label}`,
        isDirty: true,
        ...extra,
      }),
    );
    return panel;
  };
  return { ...hook, host, sessionA, sessionB, navigate, seed };
}

export function current(repo = REPO) {
  return useDockviewStore.getState().openFiles.get(buildRepoScopedItemId(PATH, repo));
}

export function start(action: () => Promise<void>) {
  let pending!: Promise<void>;
  act(() => {
    pending = action();
  });
  return pending;
}

export async function settle(
  reply: ReturnType<typeof defer<Reply>>,
  pending: Promise<void>,
  value: Reply = { success: true, new_hash: "saved-A" },
) {
  await act(async () => {
    reply.resolve(value);
    await pending;
  });
}
