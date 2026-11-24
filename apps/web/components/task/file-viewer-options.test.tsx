import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { FileViewerOptions } from "./file-viewer-options";

vi.mock("./find-file-viewer", () => ({ FindFileViewer: () => <a>Find viewer plugin</a> }));
vi.mock("./plugin-file-preview", () => ({
  PluginFilePreview: ({ onOpenText }: { onOpenText: () => void }) => (
    <button onClick={onOpenText}>Open as text</button>
  ),
}));

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("formats");
});

function openEditor(path: string) {
  return render(
    <FileViewerOptions path={path} sessionId="session">
      <textarea aria-label="Editor" />
    </FileViewerOptions>,
  );
}

it.each(["notes.txt", "main.ts"])("shows only the editor for %s", (path) => {
  const { container } = openEditor(path);
  expect(container.querySelector("textarea")).toBeTruthy();
  expect(screen.queryByText("Find viewer plugin")).toBeNull();
  expect(container.querySelector(".border-b")).toBeNull();
});

it.each(["notes.txt", "part.unknown"])("keeps edits when using Open with for %s", (path) => {
  if (path === "notes.txt") {
    pluginRegistry.setDeclaredFileViewers("formats", [
      { id: "text", title: "Text viewer", extensions: ["txt"] },
    ]);
    pluginRegistry.forPlugin("formats").registerFileViewer({ id: "text", Component: () => null });
    pluginRegistry.markPluginReady("formats", 1);
  }
  openEditor(path);
  expect(screen.queryByText("Find viewer plugin")).toBeNull();
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Unsaved edit" } });
  fireEvent.click(screen.getByRole("button", { name: "Open with" }));
  fireEvent.click(screen.getByRole("button", { name: "Open as text" }));
  expect(screen.getByDisplayValue("Unsaved edit")).toBeTruthy();
});
