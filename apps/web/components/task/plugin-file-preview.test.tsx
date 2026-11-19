import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { PluginFilePreview } from "./plugin-file-preview";
const state = vi.hoisted(() => ({
  text: "solid text",
  candidates: [] as Array<Record<string, unknown>>,
  choice: null as string | null,
}));
vi.mock("@/lib/plugins/registry", () => ({ usePluginRegistry: () => {} }));
vi.mock("@/lib/plugins/file-viewer-selection", () => ({
  fileViewerCandidates: () => state.candidates,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (value: unknown) => unknown) =>
    select({ workspaces: { activeId: "workspace" } }),
}));
vi.mock("@/hooks/use-file-viewer-choice", async () => {
  const { useState } = await import("react");
  return { useFileViewerChoice: () => useState(state.choice) };
});
vi.mock("@/hooks/use-file-operations", () => ({
  useFileOperations: () => ({ downloadFile: vi.fn() }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: async () => ({ content: state.text }),
}));
vi.mock("@/hooks/file-editor-state", () => ({
  buildFileEditorState: async () => ({ content: state.text }),
}));
vi.mock("@/hooks/use-plugin-file-preview", () => ({
  usePluginFilePreview: () => ({
    state: {
      status: "ready",
      bytes: new ArrayBuffer(1),
      selection: { size_bytes: 1, id: "selection" },
    },
  }),
}));
vi.mock("./file-viewer-header", () => ({
  FileViewerDownloadButton: () => null,
  FileViewerExternalLink: (props: unknown) => (
    <div data-testid="external-link">{JSON.stringify(props)}</div>
  ),
}));
vi.mock("./file-viewer-content", () => ({
  FileViewerContent: ({ content }: { content: string }) => <div>{content}</div>,
}));
vi.mock("./find-file-viewer", () => ({ FindFileViewer: () => null }));
vi.mock("./unavailable-file-viewers", () => ({ UnavailableFileViewers: () => null }));
vi.mock("@kandev/ui/select", () => ({
  Select: ({
    value,
    onValueChange,
    children,
  }: {
    value: string;
    onValueChange: (value: string) => void;
    children: ReactNode;
  }) => (
    <select value={value} onChange={(event) => onValueChange(event.target.value)}>
      <option value="" />
      {children}
    </select>
  ),
  SelectTrigger: () => null,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  SelectItem: ({ value, children }: { value: string; children: ReactNode }) => (
    <option value={value}>{children}</option>
  ),
}));
function viewer(id: string) {
  return {
    pluginId: id,
    id: "model",
    generation: 1,
    title: id,
    Component: () => <div>{`viewer ${id}`}</div>,
  };
}
beforeEach(() => {
  state.candidates = [viewer("a")];
  state.choice = null;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(private callback: (entries: unknown[]) => void) {}
      observe() {
        this.callback([{ isIntersecting: true }]);
      }
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("allows explicit replacement of a removed saved viewer", () => {
  state.choice = "removed/model";
  render(<PluginFilePreview path="part.stl" sessionId="session" />);
  expect(screen.queryByText("viewer a")).toBeNull();
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "a/model" } });
  expect(screen.getByText("viewer a")).toBeTruthy();
});
it.each([1, 2])("returns from text to a viewer with %i matching viewers", async (count) => {
  if (count === 2) state.candidates.push(viewer("b"));
  state.choice = "a/model";
  render(<PluginFilePreview path="part.stl" sessionId="session" />);
  fireEvent.click(screen.getByRole("button", { name: "Open as text" }));
  await screen.findByText(state.text);
  const id = count === 1 ? "a" : "b";
  fireEvent.change(screen.getByRole("combobox"), { target: { value: `${id}/model` } });
  await waitFor(() => expect(screen.queryByText(state.text)).toBeNull());
  expect(screen.getByText(`viewer ${id}`)).toBeTruthy();
});
it("keeps the repository link for unsupported files with named repository context", () => {
  state.candidates = [];
  render(<PluginFilePreview path="part.pdf" repo="models" sessionId="session" />);
  expect(JSON.parse(screen.getByTestId("external-link").textContent!)).toEqual({
    path: "part.pdf",
    repositoryName: "models",
    sessionId: "session",
  });
});
