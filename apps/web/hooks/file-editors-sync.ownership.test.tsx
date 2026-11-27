import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { useFileEditors } from "./use-file-editors";
import {
  FILE,
  REPO,
  buffer,
  expectedHash,
  panelHost,
  pauseHash,
  providers,
  publishGit,
  seed,
  selectSession,
  strictProviders,
  transport,
} from "./file-editors-sync.test-helpers";

const INITIAL_DISK = "initial disk";
const DIRECT_SESSION = "direct-session";
const REMOTE_TEXT = "remote text";
const MATCHING_TEXT = "matching text";
const NEW_REMOTE = "new remote";

let wire: ReturnType<typeof transport>;
let heldHash: ReturnType<typeof pauseHash> | undefined;
beforeEach(() => {
  useDockviewStore.getState().clearFileStates();
  wire = transport();
});
afterEach(async () => {
  await heldHash?.join();
  heldHash?.restore();
  await wire.dispose();
  heldHash = undefined;
});

describe("workspace refresh publication @covers AC-UI-FILE-EDITOR-MUTATION-001.7", () => {
  it("keeps newer acknowledged content after an older refresh completes", async () => {
    seed();
    const old = wire.sync();
    const current = wire.sync();
    await wire.reply(1, "new disk");
    await current;
    await wire.reply(0, "old disk");
    await old;
    expect(buffer().content).toBe("new disk");
  });

  it("does not publish the older read even when it completes first", async () => {
    seed();
    const old = wire.sync();
    const current = wire.sync();
    await wire.reply(0, "old disk");
    await old;
    expect(buffer().content).toBe(INITIAL_DISK);
    await wire.reply(1, "new disk");
    await current;
    expect(buffer()).toMatchObject({ content: "new disk", originalHash: expectedHash("new disk") });
  });

  it("does not resurrect older success after the newest read fails", async () => {
    seed();
    const old = wire.sync();
    const current = wire.sync();
    wire.requests[1].reject(new Error("current transport failed"));
    await current;
    await wire.reply(0, "old disk");
    await old;
    expect(buffer().content).toBe(INITIAL_DISK);
    const next = wire.sync();
    await wire.reply(2, "next disk");
    await next;
    expect(buffer().content).toBe("next disk");
  });

  it("refreshes independent paths and repositories without superseding peers", async () => {
    seed();
    seed({ path: "other.ts" });
    seed({ repo: "backend" });
    const operations = [
      wire.sync(),
      wire.sync({
        path: "other.ts",
        fileKey: buildRepoScopedItemId("other.ts", REPO),
      }),
      wire.sync({ repo: "backend", fileKey: buildRepoScopedItemId(FILE, "backend") }),
    ];
    await wire.reply(2, "backend disk");
    await wire.reply(0, "web disk");
    await wire.reply(1, "other disk");
    await Promise.all(operations);
    expect([buffer().content, buffer("other.ts").content, buffer(FILE, "backend").content]).toEqual(
      ["web disk", "other disk", "backend disk"],
    );
    expect(wire.requests.map((r) => r.payload)).toEqual([
      { session_id: DIRECT_SESSION, path: FILE, repo: REPO },
      { session_id: DIRECT_SESSION, path: "other.ts", repo: REPO },
      { session_id: DIRECT_SESSION, path: FILE, repo: "backend" },
    ]);
  });
});

describe("editor incarnation @covers AC-UI-FILE-EDITOR-MUTATION-001.8", () => {
  it("keeps an identical reopened buffer after an old refresh completes", async () => {
    seed();
    const old = wire.sync();
    useDockviewStore.getState().removeFileState(buildRepoScopedItemId(FILE, REPO));
    const reopened = seed();
    await wire.reply(0, "old disk");
    await old;
    expect(buffer()).toEqual(reopened);
  });

  it("keeps an identical replacement installed without an intervening close", async () => {
    seed();
    const old = wire.sync();
    const replacement = seed();
    await wire.reply(0, "old disk");
    await old;
    expect(buffer()).toEqual(replacement);
  });

  it("preserves the buffer when its Dockview host is replaced", async () => {
    panelHost();
    const initial = seed();
    const old = wire.sync();
    const replacement = panelHost();
    await wire.reply(0, "old disk");
    await old;
    expect(buffer()).toEqual(initial);
    expect(replacement.panel.setTitle).not.toHaveBeenCalled();
  });

  it("does not fetch or create a missing editor", async () => {
    await wire.sync();
    expect(wire.requests).toHaveLength(0);
    expect(useDockviewStore.getState().openFiles.size).toBe(0);
  });
});

describe("live reconciliation @covers AC-UI-FILE-EDITOR-MUTATION-001.9", () => {
  it("supersedes an older read while its genuine digest is pending", async () => {
    seed();
    heldHash = pauseHash("old hashed text");
    const old = wire.sync();
    wire.requests[0].resolve({ path: FILE, content: "old hashed text" });
    await heldHash.entered;
    const current = wire.sync();
    await wire.reply(1, "current disk");
    await current;
    await heldHash.join();
    await old;
    expect(buffer()).toMatchObject({
      content: "current disk",
      originalHash: expectedHash("current disk"),
    });
  });

  it("preserves typing while the real content hash is pending", async () => {
    seed();
    heldHash = pauseHash(REMOTE_TEXT);
    const pending = wire.sync();
    wire.requests[0].resolve({ path: FILE, content: REMOTE_TEXT });
    await heldHash.entered;
    useDockviewStore.getState().updateFileState(buildRepoScopedItemId(FILE, REPO), {
      content: "typing during digest",
      isDirty: true,
    });
    heldHash.release();
    await pending;
    expect(buffer()).toMatchObject({
      content: "typing during digest",
      originalContent: INITIAL_DISK,
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: REMOTE_TEXT,
      remoteOriginalHash: expectedHash(REMOTE_TEXT),
    });
  });

  it("reads matching dirty text after hashing and clears the real panel", async () => {
    const { panel } = panelHost();
    seed();
    heldHash = pauseHash(MATCHING_TEXT);
    const pending = wire.sync();
    wire.requests[0].resolve({ path: FILE, content: MATCHING_TEXT });
    await heldHash.entered;
    useDockviewStore.getState().updateFileState(buildRepoScopedItemId(FILE, REPO), {
      content: MATCHING_TEXT,
      isDirty: true,
    });
    heldHash.release();
    await pending;
    expect(buffer()).toMatchObject({
      content: MATCHING_TEXT,
      originalContent: MATCHING_TEXT,
      isDirty: false,
    });
    expect(panel.params.isDirty).toBe(false);
    expect(panel.setTitle).toHaveBeenCalledWith("refresh.ts");
  });

  it("rejects a replacement installed during hashing without touching its panel", async () => {
    const { panel } = panelHost();
    seed({ isDirty: true, content: REMOTE_TEXT, originalContent: "original" });
    heldHash = pauseHash(REMOTE_TEXT);
    const pending = wire.sync();
    wire.requests[0].resolve({ path: FILE, content: REMOTE_TEXT });
    await heldHash.entered;
    const replacement = seed();
    heldHash.release();
    await pending;
    expect(buffer()).toEqual(replacement);
    expect(panel.setTitle).not.toHaveBeenCalled();
  });
});

describe("current refresh controls @covers AC-UI-FILE-EDITOR-MUTATION-001.9", () => {
  it("accepts empty clean content with real hash and normalizes binary/resolved metadata", async () => {
    seed({ resolvedPath: "old-target", hasRemoteUpdate: true, remoteContent: "obsolete" });
    const pending = wire.sync();
    await wire.reply(0, "");
    await pending;
    expect(buffer()).toMatchObject({
      content: "",
      originalContent: "",
      originalHash: expectedHash(""),
      isBinary: false,
      resolvedPath: undefined,
      isDirty: false,
      hasRemoteUpdate: false,
      remoteContent: undefined,
    });
  });

  it("preserves already-dirty content and leaves repeated remote content as a no-op", async () => {
    seed({ content: "local draft", originalContent: INITIAL_DISK, isDirty: true });
    const first = wire.sync();
    await wire.reply(0, REMOTE_TEXT, { is_binary: true, resolved_path: "target" });
    await first;
    const prior = buffer();
    const next = wire.sync();
    await wire.reply(1, REMOTE_TEXT, { is_binary: true, resolved_path: "target" });
    await next;
    expect(buffer()).toBe(prior);
    expect(buffer()).toMatchObject({
      content: "local draft",
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: REMOTE_TEXT,
      remoteOriginalHash: expectedHash(REMOTE_TEXT),
    });
  });

  it("silently preserves content on current transport failure", async () => {
    const before = seed();
    const pending = wire.sync();
    wire.requests[0].reject(new Error("transport failed"));
    await pending;
    expect(buffer()).toBe(before);
  });

  it("retains single-repository routing and unchanged clean-content no-op", async () => {
    const before = seed({ repo: undefined });
    const pending = wire.sync({ fileKey: FILE, repo: undefined });
    await wire.reply(0, INITIAL_DISK);
    await pending;
    expect(wire.requests[0].payload).toEqual({ session_id: DIRECT_SESSION, path: FILE });
    expect(useDockviewStore.getState().openFiles.get(FILE)).toBe(before);
  });
});

function mountReader(strict = false) {
  return renderHook(() => ({ actions: useFileEditors(), store: useAppStoreApi() }), {
    wrapper: strict ? strictProviders : providers,
  });
}

describe("actual Git-status reader @covers AC-UI-FILE-EDITOR-MUTATION-001.8", () => {
  it("keeps current same-session typing and applies the actual remote reload", async () => {
    const reader = mountReader();
    selectSession(reader.result.current.store);
    const { panel } = panelHost();
    seed();
    publishGit(reader.result.current.store, "changed signature");
    expect(wire.requests).toHaveLength(1);
    act(() => reader.result.current.actions.handleFileChange(FILE, "live typing", REPO));
    await wire.reply(0, NEW_REMOTE);
    await waitFor(() => expect(buffer().remoteContent).toBe(NEW_REMOTE));
    expect(buffer().content).toBe("live typing");
    await act(() => reader.result.current.actions.applyRemoteUpdate(FILE, REPO));
    expect(buffer()).toMatchObject({
      content: NEW_REMOTE,
      originalHash: expectedHash(NEW_REMOTE),
      isDirty: false,
    });
    expect(panel.params.isDirty).toBe(false);
  });

  it("retires a committed session A-B-A read when the same key reopens", async () => {
    const reader = mountReader();
    const store = reader.result.current.store;
    const sessionA = selectSession(store);
    seed();
    publishGit(store, "A changed");
    expect(wire.requests).toHaveLength(1);
    selectSession(store);
    selectSession(store, sessionA);
    seed();
    const current = buffer();
    await wire.reply(0, "old A read");
    expect(buffer()).toEqual(current);
  });

  it.each([false, true])(
    "retires reader unmount (StrictMode=%s) without retiring a live peer",
    async (strict) => {
      const reader = mountReader(strict);
      selectSession(reader.result.current.store);
      seed();
      publishGit(reader.result.current.store, "read before disposal");
      expect(wire.requests).toHaveLength(1);
      reader.unmount();
      const before = buffer();
      await wire.reply(0, "disposed read");
      expect(buffer()).toEqual(before);
      const peer = mountReader();
      selectSession(peer.result.current.store);
      seed();
      publishGit(peer.result.current.store, "live peer signature");
      await wire.reply(1, "live peer disk");
      await waitFor(() => expect(buffer().content).toBe("live peer disk"));
    },
  );

  it("retires a null-session reader during hashing even with the same buffer still open", async () => {
    const reader = mountReader();
    selectSession(reader.result.current.store);
    seed();
    heldHash = pauseHash("old session content");
    publishGit(reader.result.current.store, "pending hash");
    wire.requests[0].resolve({ path: FILE, content: "old session content" });
    await heldHash.entered;
    act(() =>
      reader.result.current.store.setState((state) => ({
        tasks: { ...state.tasks, activeSessionId: null },
      })),
    );
    const before = buffer();
    await heldHash.join();
    await wire.joinHashes();
    expect(buffer()).toBe(before);
  });
});
