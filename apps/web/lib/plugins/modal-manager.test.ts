import { afterEach, describe, it, expect, vi } from "vitest";
import { pluginModalManager } from "./modal-manager";
import type { PluginModalOptions } from "./types";

function noopContent() {
  return null;
}

const baseOptions: PluginModalOptions = { content: noopContent };

describe("pluginModalManager", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

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

  it("captures a separate opener for each modal before publishing the snapshot", () => {
    const container = document.createElement("div");
    const openerA = document.createElement("button");
    const openerB = document.createElement("button");
    container.append(openerA, openerB);
    document.body.append(container);

    let publishedOpener: HTMLElement | undefined;
    const unsubscribe = pluginModalManager.subscribe(() => {
      const modals = pluginModalManager.getSnapshot();
      publishedOpener = modals[modals.length - 1]?.openerElement;
    });

    openerA.focus();
    const handleA = pluginModalManager.openModal("jira", baseOptions);
    expect(publishedOpener).toBe(openerA);

    openerB.focus();
    const handleB = pluginModalManager.openTaskLinkDialog("jira", baseOptions);
    expect(publishedOpener).toBe(openerB);
    expect(
      pluginModalManager
        .getSnapshot()
        .slice(-2)
        .map((modal) => modal.openerElement),
    ).toEqual([openerA, openerB]);

    unsubscribe();
    handleA.close();
    handleB.close();
    container.remove();
  });

  it("does not require a document when opened outside the browser", () => {
    vi.stubGlobal("document", undefined);

    const handle = pluginModalManager.openModal("jira", baseOptions);

    expect(pluginModalManager.getSnapshot().at(-1)?.openerElement).toBeUndefined();
    handle.close();
  });
});
