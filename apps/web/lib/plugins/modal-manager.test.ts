import { describe, it, expect } from "vitest";
import { pluginModalManager } from "./modal-manager";
import type { PluginModalOptions } from "./types";

function noopContent() {
  return null;
}

const baseOptions: PluginModalOptions = { content: noopContent };

describe("pluginModalManager", () => {
  it("open() returns a handle and adds the modal to the snapshot", () => {
    const before = pluginModalManager.getSnapshot().length;
    const handle = pluginModalManager.openModal("jira", baseOptions);

    const snapshot = pluginModalManager.getSnapshot();
    expect(snapshot).toHaveLength(before + 1);
    expect(snapshot[snapshot.length - 1].pluginId).toBe("jira");

    handle.close();
  });

  it("close() removes only the closed modal instance", () => {
    const handleA = pluginModalManager.openModal("jira", baseOptions);
    const handleB = pluginModalManager.openModal("jira", baseOptions);
    const before = pluginModalManager.getSnapshot().length;

    handleA.close();

    const snapshot = pluginModalManager.getSnapshot();
    expect(snapshot).toHaveLength(before - 1);
    expect(snapshot.some((m) => m.pluginId === "jira")).toBe(true);

    handleB.close();
  });

  it("supports multiple concurrently open modals", () => {
    const before = pluginModalManager.getSnapshot().length;
    const handleA = pluginModalManager.openModal("jira", baseOptions);
    const handleB = pluginModalManager.openModal("linear", baseOptions);

    expect(pluginModalManager.getSnapshot()).toHaveLength(before + 2);

    handleA.close();
    handleB.close();
    expect(pluginModalManager.getSnapshot()).toHaveLength(before);
  });

  it("closeAllForPlugin removes only that plugin's modals", () => {
    const jiraHandle = pluginModalManager.openModal("jira", baseOptions);
    const linearHandle = pluginModalManager.openModal("linear", baseOptions);
    const before = pluginModalManager.getSnapshot().length;

    pluginModalManager.closeAllForPlugin("jira");

    const snapshot = pluginModalManager.getSnapshot();
    expect(snapshot).toHaveLength(before - 1);
    expect(snapshot.some((m) => m.pluginId === "jira")).toBe(false);
    expect(snapshot.some((m) => m.pluginId === "linear")).toBe(true);

    void jiraHandle;
    linearHandle.close();
  });

  it("generates monotonically increasing, unique instance ids", () => {
    const handleA = pluginModalManager.openModal("jira", baseOptions);
    const snapshotA = pluginModalManager.getSnapshot();
    const idA = snapshotA[snapshotA.length - 1].instanceId;

    const handleB = pluginModalManager.openModal("jira", baseOptions);
    const snapshotB = pluginModalManager.getSnapshot();
    const idB = snapshotB[snapshotB.length - 1].instanceId;

    expect(idA).not.toBe(idB);

    handleA.close();
    handleB.close();
  });

  it("close() is a no-op when the modal is already closed", () => {
    const handle = pluginModalManager.openModal("jira", baseOptions);
    handle.close();
    const before = pluginModalManager.getSnapshot().length;
    expect(() => handle.close()).not.toThrow();
    expect(pluginModalManager.getSnapshot()).toHaveLength(before);
  });
});
