import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { modelUriForDocument } from "@/lib/lsp/file-uri";

const mocks = vi.hoisted(() => ({
  getWorkspaceUriForSession: vi.fn(() => "file:///workspace"),
  promoteDocumentModel: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeSessionId: string | null } }) => unknown) =>
    selector({ tasks: { activeSessionId: null } }),
}));

vi.mock("@/hooks/use-lsp", () => ({
  useLsp: () => ({ status: { state: "disabled" }, lspLanguage: null, toggle: vi.fn() }),
}));

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    getWorkspaceUriForSession: mocks.getWorkspaceUriForSession,
    promoteDocumentModel: mocks.promoteDocumentModel,
  },
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { useMonacoEditorLsp } from "./use-monaco-editor-lsp";

describe("useMonacoEditorLsp model identity", () => {
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
});
