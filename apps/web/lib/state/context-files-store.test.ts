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

describe("context files store", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    useContextFilesStore.setState({ filesBySessionId: {} });
  });

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
