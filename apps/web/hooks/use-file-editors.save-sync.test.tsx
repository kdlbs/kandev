import {
  mocks,
  PATH,
  REPO,
  type Reply,
  defer,
  setup,
  current,
  start,
  settle,
  panelHost,
} from "./file-editor-mutation.test-helpers";
import { act, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import * as fileDiff from "@/lib/utils/file-diff";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { syncOpenFileFromWorkspace } from "./file-editors-sync";

const AGENT_CONTENT = "newer agent content";
const TYPED_CONTENT = "later typing";
const ACCEPTED_CONTENT = "accepted agent content";

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  useDockviewStore.getState().clearFileStates();
  useDockviewStore.setState({ api: null, isRestoringLayout: false });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  useDockviewStore.setState({ api: null });
});

async function refresh(h: ReturnType<typeof setup>, content: string, repo = REPO) {
  vi.mocked(requestFileContent).mockResolvedValueOnce({
    path: PATH,
    size: content.length,
    content,
    is_binary: false,
  });
  await act(async () => {
    await syncOpenFileFromWorkspace({
      client: mocks.client as never,
      sessionId: h.sessionA,
      fileKey: buildRepoScopedItemId(PATH, repo),
      path: PATH,
      repo,
      updateFileState: useDockviewStore.getState().updateFileState,
      isCurrent: () => true,
    });
  });
}

// @covers AC-UI-FILE-EDITOR-MUTATION-001.10
it("retains a distinct workspace snapshot when an older save receipt settles", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, AGENT_CONTENT);
  const remoteHash = await fileDiff.calculateHash(AGENT_CONTENT);
  expect(current()).toMatchObject({ hasRemoteUpdate: true, remoteContent: AGENT_CONTENT });
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: "A buffer",
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: false,
    hasRemoteUpdate: true,
    remoteContent: AGENT_CONTENT,
    remoteOriginalHash: remoteHash,
  });
  await act(async () => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  expect(current()).toMatchObject({
    content: AGENT_CONTENT,
    originalContent: AGENT_CONTENT,
    originalHash: remoteHash,
    isDirty: false,
    hasRemoteUpdate: false,
  });
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.10
it.each(["before", "during"])("clears a matching save echo observed %s save", async (when) => {
  const h = setup();
  h.seed();
  if (when === "before") await refresh(h, "preexisting agent content");
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, "A buffer");
  act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: TYPED_CONTENT,
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: true,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  });
});

it("resolves a preexisting conflict on successful save without a new observation", async () => {
  const h = setup();
  h.seed();
  await refresh(h, "preexisting agent content");
  mocks.save.mockResolvedValueOnce({ success: true, new_hash: "saved-A" });
  await act(async () => h.result.current.actions.saveFile(PATH, REPO));
  expect(current()).toMatchObject({
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: false,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  });
});

it.each(["", AGENT_CONTENT])("preserves typing and newer workspace content %j", async (agent) => {
  const h = setup();
  const panel = h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, agent);
  act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: TYPED_CONTENT,
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: true,
    hasRemoteUpdate: true,
    remoteContent: agent,
    remoteOriginalHash: await fileDiff.calculateHash(agent),
  });
  expect(panel.params.isDirty).toBe(true);
  expect(mocks.lsp).toHaveBeenCalledWith(h.sessionA, PATH, REPO, "A buffer", TYPED_CONTENT);
});

it.each([false, true])("retains refresh-accepted baseline with later typing=%s", async (typing) => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, "A buffer");
  await refresh(h, ACCEPTED_CONTENT);
  if (typing) act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: typing ? TYPED_CONTENT : ACCEPTED_CONTENT,
    originalContent: ACCEPTED_CONTENT,
    originalHash: await fileDiff.calculateHash(ACCEPTED_CONTENT),
    isDirty: typing,
    hasRemoteUpdate: false,
  });
  expect(mocks.lsp).toHaveBeenCalledWith(
    h.sessionA,
    PATH,
    REPO,
    "A buffer",
    typing ? TYPED_CONTENT : ACCEPTED_CONTENT,
  );
});

it.each([false, true])(
  "keeps normal receipt-before-refresh behavior with typing=%s",
  async (typing) => {
    const h = setup();
    h.seed();
    const reply = defer<Reply>();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
    if (typing) act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
    await settle(reply, pending);
    await refresh(h, AGENT_CONTENT);
    expect(current()).toMatchObject(
      typing
        ? {
            content: TYPED_CONTENT,
            originalContent: "A buffer",
            originalHash: "saved-A",
            isDirty: true,
            hasRemoteUpdate: true,
            remoteContent: AGENT_CONTENT,
          }
        : {
            content: AGENT_CONTENT,
            originalContent: AGENT_CONTENT,
            originalHash: await fileDiff.calculateHash(AGENT_CONTENT),
            isDirty: false,
            hasRemoteUpdate: false,
          },
    );
  },
);

it.each(["response", "transport"])(
  "preserves newer remote state on save %s failure",
  async (failure) => {
    const h = setup();
    h.seed();
    const reply = defer<Reply>();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
    await refresh(h, AGENT_CONTENT);
    const snapshot = current();
    if (failure === "response")
      await settle(reply, pending, { success: false, error: "write failed" });
    else
      await act(async () => {
        reply.reject(new Error("connection lost"));
        await pending;
      });
    expect(current()).toEqual(snapshot);
    expect(mocks.lsp).not.toHaveBeenCalled();
    expect(mocks.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  },
);

it("does not let another repository's refresh preserve a preexisting warning", async () => {
  const h = setup();
  h.seed();
  h.seed("backend", "backend");
  await refresh(h, "old warning");
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, "backend agent content", "backend");
  const other = current("backend");
  await settle(reply, pending);
  expect(current()?.hasRemoteUpdate).toBe(false);
  expect(current("backend")).toEqual(other);
});

it("rechecks the selected remote snapshot after deferred reload hashing", async () => {
  const h = setup();
  h.seed("A", REPO, false, { hasRemoteUpdate: true, remoteContent: "old remote" });
  const hash = defer<string>();
  vi.spyOn(fileDiff, "calculateHash").mockReturnValueOnce(hash.promise);
  const pending = start(() => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  await refresh(h, AGENT_CONTENT);
  const newer = current();
  await act(async () => {
    hash.resolve("old-hash");
    await pending;
  });
  expect(current()).toEqual(newer);
  await act(async () => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  expect(current()?.content).toBe(AGENT_CONTENT);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.10
it("clears an intervening agent warning when the latest observation is the save echo", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, AGENT_CONTENT);
  await refresh(h, "A buffer");
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: "A buffer",
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: false,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  });
});

it.each(["navigation", "reopen", "host"] as const)(
  "cannot carry an observation across %s ownership retirement",
  async (retirement) => {
    const h = setup();
    const oldPanel = h.seed();
    const reply = defer<Reply>();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
    await refresh(h, AGENT_CONTENT);
    if (retirement === "navigation") h.navigate(h.sessionB);
    if (retirement === "reopen") act(() => h.host.api.removePanel(oldPanel));
    if (retirement === "host") act(() => useDockviewStore.setState({ api: panelHost().dockApi }));
    h.seed("replacement");
    const replacement = current();
    await settle(reply, pending, { success: true, new_hash: "saved-A", resolution: "overwritten" });
    expect(current()).toEqual(replacement);
    expect(mocks.lsp).not.toHaveBeenCalled();
    expect(mocks.toast).not.toHaveBeenCalled();
  },
);

// @covers AC-UI-FILE-EDITOR-MUTATION-001.10
it("records the saved baseline when a conflicting observation matches the original disk bytes", async () => {
  const h = setup();
  const originalContent = "A disk";
  const originalHash = await fileDiff.calculateHash(originalContent);
  h.seed("A", REPO, false, { originalHash });
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, originalContent);
  expect(current()).toMatchObject({ hasRemoteUpdate: true, remoteOriginalHash: originalHash });
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: "A buffer",
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: false,
    hasRemoteUpdate: true,
    remoteContent: originalContent,
    remoteOriginalHash: originalHash,
  });
  act(() => h.result.current.actions.handleFileChange(PATH, originalContent, REPO));
  expect(current()?.isDirty).toBe(true);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.10
it.each([
  ["reload", ACCEPTED_CONTENT],
  ["reload", "A disk"],
  ["refresh", ACCEPTED_CONTENT],
  ["refresh", "A disk"],
] as const)(
  "retains a baseline accepted by %s (%s) beneath a later warning",
  async (mode, accepted) => {
    const h = setup();
    const originalHash = await fileDiff.calculateHash("A disk");
    h.seed("A", REPO, false, { originalHash });
    const reply = defer<Reply>();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
    if (mode === "refresh") await refresh(h, "A buffer");
    await refresh(h, accepted);
    if (mode === "reload")
      await act(async () => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
    act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
    await refresh(h, AGENT_CONTENT);
    const baseline = {
      originalContent: accepted,
      originalHash: await fileDiff.calculateHash(accepted),
    };
    expect(current()).toMatchObject({ ...baseline, hasRemoteUpdate: true });
    await settle(reply, pending);
    expect(current()).toMatchObject({
      ...baseline,
      content: TYPED_CONTENT,
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: AGENT_CONTENT,
      remoteOriginalHash: await fileDiff.calculateHash(AGENT_CONTENT),
    });
  },
);

it("preserves explicit Reload acceptance of a preexisting snapshot during save", async () => {
  const h = setup();
  h.seed();
  await refresh(h, ACCEPTED_CONTENT);
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await act(async () => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: ACCEPTED_CONTENT,
    originalContent: ACCEPTED_CONTENT,
    originalHash: await fileDiff.calculateHash(ACCEPTED_CONTENT),
    isDirty: false,
    hasRemoteUpdate: false,
  });
});

it("records the save when a later matching echo supersedes a previously accepted baseline", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await refresh(h, "A buffer");
  await refresh(h, ACCEPTED_CONTENT);
  act(() => h.result.current.actions.handleFileChange(PATH, TYPED_CONTENT, REPO));
  await refresh(h, "A buffer");
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: TYPED_CONTENT,
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: true,
    hasRemoteUpdate: false,
    remoteContent: undefined,
  });
});
