import { StrictMode } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useAppStoreApi } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useEditorResolverStore } from "@/lib/state/editor-resolver-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import {
  FILE,
  REPO,
  buffer,
  expectedHash,
  panelHost,
  providers,
  publishGit,
  seed,
  selectSession,
  transport,
} from "@/hooks/file-editors-sync.test-helpers";
import { FileEditorPanel } from "./file-editor-panel";

const EDITOR_CONTEXT = "code-editor";
const LOCAL_DRAFT = "unsaved typing";
const REMOTE_TEXT = "remote text";

let wire: ReturnType<typeof transport>;
let registeredPanel: string | undefined;
let previousEditor = useEditorResolverStore.getState().getProvider(EDITOR_CONTEXT);
beforeEach(() => {
  previousEditor = useEditorResolverStore.getState().getProvider(EDITOR_CONTEXT);
  useEditorResolverStore.getState().setProvider(EDITOR_CONTEXT, "codemirror");
  wire = transport();
});
afterEach(async () => {
  await wire.dispose();
  if (registeredPanel) panelPortalManager.release(registeredPanel);
  registeredPanel = undefined;
  useEditorResolverStore.getState().setProvider(EDITOR_CONTEXT, previousEditor);
});

function mountPanel(active = false, strict = false, dirty = false) {
  let store!: ReturnType<typeof useAppStoreApi>;
  const host = panelHost(FILE, REPO, active);
  function Probe() {
    store = useAppStoreApi();
    useFileEditors();
    return null;
  }
  const tree = (visible: boolean) => {
    const body = providers({
      children: (
        <CommandRegistryProvider>
          <Probe />
          {visible && (
            <FileEditorPanel panelId={host.panel.id} params={{ path: FILE, repo: REPO }} />
          )}
        </CommandRegistryProvider>
      ),
    });
    return strict ? <StrictMode>{body}</StrictMode> : body;
  };
  const root = render(tree(false));
  const session = selectSession(store);
  seed(
    dirty
      ? { content: LOCAL_DRAFT, originalContent: "initial disk", isDirty: true }
      : { isBinary: true },
  );
  panelPortalManager.acquire(host.panel.id, "file-editor", host.panel.params, host.api);
  registeredPanel = host.panel.id;
  root.rerender(tree(true));
  return { ...host, store, session, hide: () => root.rerender(tree(false)), root };
}

describe("real panel workspace callbacks @covers AC-UI-FILE-EDITOR-MUTATION-001.7", () => {
  it("refreshes an initially active panel and preserves genuine binary normalization", async () => {
    mountPanel(true);
    expect(wire.requests).toHaveLength(1);
    await wire.reply(0, "new disk", { is_binary: true });
    expect(buffer()).toMatchObject({
      content: "new disk",
      originalContent: "new disk",
      originalHash: expectedHash("new disk"),
      isBinary: true,
    });
  });

  it("refreshes only true activation through the registered callback", async () => {
    const panel = mountPanel();
    expect(wire.requests).toHaveLength(0);
    panel.activate(false);
    expect(wire.requests).toHaveLength(0);
    panel.activate();
    expect(wire.requests).toHaveLength(1);
    await wire.reply(0, "activated disk", { is_binary: true });
    expect(buffer().content).toBe("activated disk");
  });

  it("shares latest-read ordering across actual Git and activation consumers", async () => {
    const panel = mountPanel(true);
    publishGit(panel.store, "new Git signature");
    expect(wire.requests).toHaveLength(3); // Probe and panel each own a Git reader.
    panel.activate();
    expect(wire.requests).toHaveLength(4);
    await wire.reply(3, "latest activation", { is_binary: true });
    for (const index of [0, 2, 1]) await wire.reply(index, "superseded disk", { is_binary: true });
    expect(buffer()).toMatchObject({
      content: "latest activation",
      originalHash: expectedHash("latest activation"),
    });
  });
});

describe("real activation lifetime @covers AC-UI-FILE-EDITOR-MUTATION-001.8", () => {
  it("rejects an activation reply after panel consumer unmount with the same buffer intact", async () => {
    const panel = mountPanel(true);
    const before = buffer();
    panel.hide();
    expect(panel.listeners.size).toBe(0);
    await wire.reply(0, "unmounted activation", { is_binary: true });
    expect(buffer()).toBe(before);
    expect(panel.panel.setTitle).not.toHaveBeenCalled();
  });

  it("cannot admit a captured retired callback after disposal", async () => {
    const panel = mountPanel();
    const callback = [...panel.listeners][0];
    expect(callback).toBeTypeOf("function");
    panel.hide();
    publishGit(panel.store, "live peer read");
    expect(wire.requests).toHaveLength(1);
    act(() => callback({ isActive: true }));
    expect(wire.requests).toHaveLength(1);
    await wire.reply(0, "live peer disk", { is_binary: true });
    expect(buffer().content).toBe("live peer disk");
  });

  it("rejects the old portal API after replacement without remounting the panel", async () => {
    const panel = mountPanel(true);
    const before = buffer();
    const replacement = panelHost();
    // Keep the Dockview host unchanged to isolate the portal API boundary.
    act(() => useDockviewStore.setState({ api: panel.host }));
    panelPortalManager.acquire(panel.panel.id, "file-editor", panel.panel.params, replacement.api);
    await wire.reply(0, "retired portal", { is_binary: true });
    expect(buffer()).toBe(before);
    panel.activate();
    expect(wire.requests).toHaveLength(1);
  });

  it("keeps only the live StrictMode activation publication", async () => {
    const panel = mountPanel(true, true);
    expect(wire.requests).toHaveLength(2);
    expect(panel.listeners.size).toBe(1);
    await wire.reply(1, "live replay", { is_binary: true });
    await wire.reply(0, "retired replay", { is_binary: true });
    expect(buffer().content).toBe("live replay");
  });
});

describe("actual dirty panel affordance @covers AC-UI-FILE-EDITOR-MUTATION-001.9", () => {
  it("retains unsaved text and applies remote content through the rendered Reload action", async () => {
    const panel = mountPanel(false, false, true);
    panel.activate();
    await wire.reply(0, REMOTE_TEXT);
    expect(buffer()).toMatchObject({
      content: LOCAL_DRAFT,
      isDirty: true,
      hasRemoteUpdate: true,
    });
    const reload = await screen.findByRole("button", { name: "Reload" });
    act(() => reload.click());
    await waitFor(() => expect(buffer().isDirty).toBe(false));
    expect(buffer()).toMatchObject({
      content: REMOTE_TEXT,
      originalContent: REMOTE_TEXT,
      originalHash: expectedHash(REMOTE_TEXT),
      hasRemoteUpdate: false,
    });
    expect(panel.panel.params.isDirty).toBe(false);
    expect(panel.panel.setTitle).toHaveBeenCalledWith("refresh.ts");
  });

  it("clears matching dirty text and the panel title without overwriting its buffer", async () => {
    const panel = mountPanel(false, false, true);
    panel.activate();
    await wire.reply(0, LOCAL_DRAFT);
    expect(buffer()).toMatchObject({
      content: LOCAL_DRAFT,
      isDirty: false,
      originalHash: expectedHash(LOCAL_DRAFT),
    });
    expect(panel.panel.params.isDirty).toBe(false);
    expect(panel.panel.setTitle).toHaveBeenCalledWith("refresh.ts");
  });

  it("preserves typing made after an active read started", async () => {
    mountPanel(true);
    act(() =>
      useDockviewStore.getState().updateFileState(buildRepoScopedItemId(FILE, REPO), {
        content: "typed after activation",
        isDirty: true,
      }),
    );
    await wire.reply(0, "new remote", { is_binary: true });
    expect(buffer()).toMatchObject({
      content: "typed after activation",
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: "new remote",
    });
  });
});
