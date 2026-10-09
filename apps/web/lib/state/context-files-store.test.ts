import { beforeEach, describe, expect, it } from "vitest";
import { useContextFilesStore, type ContextFile } from "./context-files-store";

const SESSION_ID = "session-context-1";
const STORAGE_KEY = `kandev.contextFiles.${SESSION_ID}`;

function directory(path: string, pinned = false): ContextFile {
  return { path, name: path.split("/").at(-1) ?? path, isDirectory: true, pinned } as ContextFile;
}

function file(path: string, pinned = false): ContextFile {
  return { path, name: path.split("/").at(-1) ?? path, pinned };
}

beforeEach(() => {
  window.sessionStorage.clear();
  useContextFilesStore.setState({ filesBySessionId: {} });
});

describe("context files store", () => {
  it("round-trips directory identity through session storage", () => {
    const attached = directory("src/components");

    useContextFilesStore.getState().addFile(SESSION_ID, attached);
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify([attached]));

    useContextFilesStore.setState({ filesBySessionId: {} });
    useContextFilesStore.getState().hydrateSession(SESSION_ID);

    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([attached]);
  });

  it("keeps legacy file-only session entries readable", () => {
    const legacy = file("README.md");
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify([legacy]));

    useContextFilesStore.getState().hydrateSession(SESSION_ID);

    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([legacy]);
  });

  it("gives a removed and re-added descriptor a new selection identity", () => {
    const descriptor = file("same.ts");
    const store = useContextFilesStore.getState();
    store.addFile(SESSION_ID, descriptor);
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID][0];
    store.removeFile(SESSION_ID, descriptor.path);
    store.addFile(SESSION_ID, descriptor);
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID][0]).not.toBe(submitted);
  });

  it("deduplicates files and directories by path and clears only ephemeral entries", () => {
    const pinnedFile = file("README.md", true);
    const ephemeralDirectory = directory("src");

    useContextFilesStore.getState().addFile(SESSION_ID, ephemeralDirectory);
    useContextFilesStore.getState().addFile(SESSION_ID, directory("src"));
    useContextFilesStore.getState().addFile(SESSION_ID, pinnedFile);

    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([
      ephemeralDirectory,
      pinnedFile,
    ]);

    useContextFilesStore.getState().clearEphemeral(SESSION_ID);

    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([pinnedFile]);
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify([pinnedFile]));
  });
});

describe("submitted context consumption", () => {
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.11
  it("consumes only submitted selections and persists later files and directories", () => {
    const store = useContextFilesStore.getState();
    store.addFile(SESSION_ID, file("old.ts"));
    store.addFile(SESSION_ID, directory("old-dir"));
    store.addFile(SESSION_ID, file("pinned.ts", true));
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID];
    store.addFile(SESSION_ID, file("new.ts"));
    store.addFile(SESSION_ID, directory("new-dir"));
    store.addFile("other-session", file("other.ts"));

    store.consumeSubmittedEphemeral(SESSION_ID, submitted);

    const expected = [file("pinned.ts", true), file("new.ts"), directory("new-dir")];
    expect(JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!)).toEqual(expected);
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual(expected);
    expect(useContextFilesStore.getState().filesBySessionId["other-session"]).toEqual([
      file("other.ts"),
    ]);
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.12
  it("preserves replacement selections but consumes a duplicate no-op selection", () => {
    const store = useContextFilesStore.getState();
    const replacement = file("same.ts");
    store.addFile(SESSION_ID, replacement);
    store.addFile(SESSION_ID, file("duplicate.ts"));
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID];
    store.removeFile(SESSION_ID, replacement.path);
    store.addFile(SESSION_ID, replacement);
    store.addFile(SESSION_ID, file("duplicate.ts"));
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID][0]).toBe(submitted[1]);

    store.consumeSubmittedEphemeral(SESSION_ID, submitted);

    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([replacement]);
    expect(JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!)).toEqual([replacement]);
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.14
  it("retains changed pin state and current metadata", () => {
    const store = useContextFilesStore.getState();
    store.addFile(SESSION_ID, file("pin.ts"));
    store.addFile(SESSION_ID, file("unpin.ts", true));
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID];
    store.addFile(SESSION_ID, file("pin.ts", true));
    store.unpinFile(SESSION_ID, "unpin.ts");
    store.consumeSubmittedEphemeral(SESSION_ID, submitted);
    const expected = [file("pin.ts", true), file("unpin.ts")];
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual(expected);
    expect(JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!)).toEqual(expected);
  });

  it("consumes legacy, prompt and plan entries without changing their persisted shape", () => {
    window.sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify([{ path: "legacy.ts", name: "legacy.ts" }]),
    );
    const store = useContextFilesStore.getState();
    store.hydrateSession(SESSION_ID);
    store.addFile(SESSION_ID, file("prompt:review"));
    store.addFile(SESSION_ID, file("plan:context", true));
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID];
    store.consumeSubmittedEphemeral(SESSION_ID, submitted);
    expect(JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!)).toEqual([
      file("plan:context", true),
    ]);
    store.clearSession(SESSION_ID);
    store.hydrateSession(SESSION_ID);
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([
      file("plan:context", true),
    ]);
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.13
  it("does not restore removed context or change state for repeated and empty consumption", () => {
    const store = useContextFilesStore.getState();
    store.addFile(SESSION_ID, file("old.ts"));
    const submitted = useContextFilesStore.getState().filesBySessionId[SESSION_ID];
    store.removeFile(SESSION_ID, "old.ts");
    store.addFile(SESSION_ID, file("next.ts"));
    const current = useContextFilesStore.getState();
    store.consumeSubmittedEphemeral(SESSION_ID, submitted);
    store.consumeSubmittedEphemeral(SESSION_ID, []);
    expect(useContextFilesStore.getState()).toBe(current);
    store.clearEphemeral(SESSION_ID);
    store.consumeSubmittedEphemeral(SESSION_ID, submitted);
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toEqual([]);
    store.clearSession(SESSION_ID);
    store.consumeSubmittedEphemeral(SESSION_ID, submitted);
    expect(useContextFilesStore.getState().filesBySessionId[SESSION_ID]).toBeUndefined();
  });
});
