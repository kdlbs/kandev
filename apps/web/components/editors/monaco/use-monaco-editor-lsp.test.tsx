import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { modelUriForDocument } from "@/lib/lsp/file-uri";

const mocks = vi.hoisted(() => ({
  getWorkspaceUriForSession: vi.fn(() => "file:///workspace"),
  promoteDocumentModel: vi.fn(),
  openDocument: vi.fn(),
  closeDocument: vi.fn(),
  useLsp: vi.fn((_sessionId: string | null, _language: string) => undefined),
  lspStatus: { state: "disabled" as "disabled" | "ready" | "reconnecting" },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeSessionId: string | null } }) => unknown) =>
    selector({ tasks: { activeSessionId: null } }),
}));

vi.mock("@/hooks/use-lsp", () => ({
  useLsp: (sessionId: string | null, language: string) => {
    mocks.useLsp(sessionId, language);
    return {
      status: sessionId ? mocks.lspStatus : { state: "disabled" },
      lspLanguage: "typescript",
      toggle: vi.fn(),
    };
  },
}));

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    getWorkspaceUriForSession: mocks.getWorkspaceUriForSession,
    promoteDocumentModel: mocks.promoteDocumentModel,
    openDocument: mocks.openDocument,
    closeDocument: mocks.closeDocument,
  },
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { useMonacoEditorLsp } from "./use-monaco-editor-lsp";

describe("useMonacoEditorLsp model identity", () => {
  it("keeps the session model scoped without starting LSP when disabled", () => {
    const contentRef = { current: "class Target {}" };
    const editorRef = { current: null };
    const { result } = renderHook(() =>
      useMonacoEditorLsp({
        sessionId: "session",
        worktreePath: "/host/worktree",
        language: "kotlin",
        path: "src/Target.kt",
        contentRef,
        editorRef,
        editorReady: true,
        enableLsp: false,
      }),
    );

    expect(mocks.useLsp).toHaveBeenCalledWith(null, "kotlin");
    expect(mocks.promoteDocumentModel).not.toHaveBeenCalled();
    expect(mocks.openDocument).not.toHaveBeenCalled();
    expect(result.current.lspStatus.state).toBe("disabled");
  });

  it("keeps the cached task-host URI and promotes files without an active LSP", () => {
    const contentRef = { current: "class Target {}" };
    const editorRef = { current: null };
    const { result } = renderHook(() =>
      useMonacoEditorLsp({
        sessionId: "session",
        worktreePath: "/host/worktree",
        language: "java",
        path: "src/Target.java",
        contentRef,
        editorRef,
        editorReady: true,
      }),
    );
    const documentUri = "file:///workspace/src/Target.java";

    expect(result.current.monacoPath).toBe(modelUriForDocument(documentUri, "session"));
    expect(mocks.promoteDocumentModel).toHaveBeenCalledWith(
      "session",
      documentUri,
      contentRef.current,
    );
  });

  it("keeps its document registration through a transient reconnect", () => {
    mocks.lspStatus.state = "ready";
    const contentRef = { current: "class Target {}" };
    const editorRef = { current: null };
    const { rerender, unmount } = renderHook(() =>
      useMonacoEditorLsp({
        sessionId: "session",
        worktreePath: "/host/worktree",
        language: "typescript",
        path: "src/Target.ts",
        contentRef,
        editorRef,
        editorReady: true,
      }),
    );

    expect(mocks.openDocument).toHaveBeenCalledTimes(1);
    mocks.lspStatus.state = "reconnecting";
    rerender();
    expect(mocks.closeDocument).not.toHaveBeenCalled();

    unmount();
    expect(mocks.closeDocument).toHaveBeenCalledTimes(1);
  });
});
