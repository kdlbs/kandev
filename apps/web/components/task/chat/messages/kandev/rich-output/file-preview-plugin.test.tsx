import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { FilePreview } from "./file-preview-block";
const read = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: read }));
vi.mock("@/components/task/plugin-file-preview", () => ({
  PluginFilePreview: ({
    path,
    repo,
    sessionId,
  }: {
    path: string;
    repo?: string;
    sessionId: string;
  }) => <div data-testid="shared-preview">{JSON.stringify({ path, repo, sessionId })}</div>,
}));
const PLUGIN_ID = "custom";
const SHARED_PREVIEW = "shared-preview";
const MODEL_PATH = "part.shape";
function register() {
  pluginRegistry.setDeclaredFileViewers(PLUGIN_ID, [
    { id: "model", title: "Model", extensions: ["shape"] },
  ]);
  pluginRegistry.forPlugin(PLUGIN_ID).registerFileViewer({ id: "model", Component: () => null });
  pluginRegistry.markPluginReady(PLUGIN_ID, 1);
}
afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.clearAllMocks();
});
it("routes future formats to the shared preview without reading them as text", () => {
  register();
  render(<FilePreview path={MODEL_PATH} repo="models" sessionId="session" />);
  expect(screen.getByTestId(SHARED_PREVIEW).textContent).toBe(
    JSON.stringify({ path: MODEL_PATH, repo: "models", sessionId: "session" }),
  );
  expect(read).not.toHaveBeenCalled();
});
it("unmounts plugin previews when collapsed and preserves their route after uninstall", () => {
  register();
  render(<FilePreview path={MODEL_PATH} sessionId="session" />);
  act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
  expect(screen.getByTestId(SHARED_PREVIEW)).toBeTruthy();
  fireEvent.click(screen.getByTestId("rich-output-file-preview-toggle"));
  expect(screen.queryByTestId(SHARED_PREVIEW)).toBeNull();
  expect(read).not.toHaveBeenCalled();
});
it("does not give a plugin a preview without a session", () => {
  register();
  render(<FilePreview path={MODEL_PATH} />);
  expect(screen.queryByTestId(SHARED_PREVIEW)).toBeNull();
  expect(read).not.toHaveBeenCalled();
});
