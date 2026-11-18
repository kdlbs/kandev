import { afterEach, expect, it } from "vitest";
import { pluginRegistry } from "./registry";
import {
  shouldOpenFilePreview,
  canOfferViewerAfterReadFailure,
  fileViewerCandidates,
} from "./file-viewer-selection";
afterEach(() => pluginRegistry.unregisterPlugin("formats"));
it("preserves built-in and source defaults while bypassing text reads for binary formats", () => {
  pluginRegistry.setDeclaredFileViewers("formats", [
    { id: "all", title: "Formats", extensions: ["stl", "ts", "png", "md"] },
  ]);
  pluginRegistry.forPlugin("formats").registerFileViewer({ id: "all", Component: () => null });
  pluginRegistry.markPluginReady("formats", 1);
  expect(shouldOpenFilePreview("Model.STL")).toBe(true);
  expect(shouldOpenFilePreview("book.pdf")).toBe(true);
  for (const path of ["file.ts", "file.png", "file.md"])
    expect(shouldOpenFilePreview(path)).toBe(false);
  expect(fileViewerCandidates("file.ts")).toHaveLength(1);
  expect(fileViewerCandidates("file.png")).toHaveLength(0);
  expect(canOfferViewerAfterReadFailure("large.custom-format")).toBe(true);
  expect(canOfferViewerAfterReadFailure("source.ts")).toBe(false);
});
it("extracts the preview name from Windows paths", async () => {
  const { previewFileState } = await import("./file-viewer-selection");
  expect(previewFileState("C:\\models\\part.stl").name).toBe("part.stl");
});
