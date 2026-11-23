import { afterEach, expect, it } from "vitest";
import { pluginRegistry } from "./registry";
const id = "viewer-test";
afterEach(() => pluginRegistry.unregisterPlugin(id));
it("rejects undeclared viewers and uses only manifest formats", () => {
  const api = pluginRegistry.forPlugin(id);
  expect(() => api.registerFileViewer({ id: "stl", Component: () => null })).toThrow();
  pluginRegistry.setDeclaredFileViewers(id, [{ id: "stl", title: "STL", extensions: ["stl"] }]);
  api.registerFileViewer({ id: "stl", Component: () => null });
  expect(pluginRegistry.getFileViewers()).toHaveLength(0);
  pluginRegistry.markPluginReady(id, 1);
  expect(pluginRegistry.getFileViewers()[0]).toMatchObject({
    pluginId: id,
    id: "stl",
    extensions: ["stl"],
    generation: 1,
  });
  expect(() => api.registerFileViewer({ id: "stl", Component: () => null })).toThrow();
  pluginRegistry.unregisterPlugin(id);
  expect(pluginRegistry.getFileViewers()).toHaveLength(0);
});
it("rolls back a failed registration transaction", () => {
  pluginRegistry.setDeclaredFileViewers(id, [{ id: "stl", title: "STL", extensions: ["stl"] }]);
  expect(() =>
    pluginRegistry.runAtomicMutation(() => {
      pluginRegistry.forPlugin(id).registerFileViewer({ id: "stl", Component: () => null });
      throw Error("rollback");
    }),
  ).toThrow();
  pluginRegistry.markPluginReady(id, 1);
  expect(pluginRegistry.getFileViewers()).toHaveLength(0);
});
