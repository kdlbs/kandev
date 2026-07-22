import { describe, expect, it } from "vitest";
import { Uri as MonacoUri } from "monaco-editor";
import {
  canonicalFileUri,
  filePathToUri,
  fileUrisEqual,
  isSessionModelUri,
  joinFileUri,
  modelUriForDocument,
  documentUriForModel,
  resolveFileUriInWorkspace,
} from "./file-uri";

const POSIX_WORKSPACE_URI = "file:///workspace";
const ENCODED_WORKSPACE_URI = "file:///task%20root";
const WINDOWS_WORKSPACE_URI = "file:///C:/workspace";
const WINDOWS_TASK_ROOT_URI = "file:///C:/Task%20Root";
const UNC_WORKSPACE_URI = "file://build-server/work%20share";
const BACKEND_REPOSITORY = "backend";
const KOTLIN_SOURCE_PATH = "src/Main.kt";

describe("LSP file URI construction", () => {
  it.each([
    ["/task root/src/A#B?100%.kt", "file:///task%20root/src/A%23B%3F100%25.kt"],
    ["C:\\Task Root\\src\\Main.kt", "file:///C:/Task%20Root/src/Main.kt"],
    ["\\\\build-server\\work share\\Main.kt", "file://build-server/work%20share/Main.kt"],
    ["/workspace/src/Olá.kt", "file:///workspace/src/Ol%C3%A1.kt"],
    ["/task:root/A!B(C):D.kt", "file:///task%3Aroot/A%21B%28C%29%3AD.kt"],
  ])("converts task-host path %s to a canonical URI", (path, expected) => {
    expect(filePathToUri(path)).toBe(expected);
  });

  it("joins repository-scoped paths onto an encoded workspace URI", () => {
    expect(joinFileUri(ENCODED_WORKSPACE_URI, "orders api", "src/A#B?.kt")).toBe(
      "file:///task%20root/orders%20api/src/A%23B%3F.kt",
    );
    expect(joinFileUri(ENCODED_WORKSPACE_URI, undefined, "orders api/src/A#B?.kt")).toBe(
      "file:///task%20root/orders%20api/src/A%23B%3F.kt",
    );
  });

  it("preserves POSIX backslashes and normalizes Windows separators", () => {
    const posixUri = joinFileUri(POSIX_WORKSPACE_URI, undefined, String.raw`src/A\B.kt`);
    expect(posixUri).toBe("file:///workspace/src/A%5CB.kt");
    expect(resolveFileUriInWorkspace(posixUri, POSIX_WORKSPACE_URI)).toEqual({
      path: String.raw`src/A\B.kt`,
    });
    expect(joinFileUri(WINDOWS_WORKSPACE_URI, undefined, String.raw`src\Main.kt`)).toBe(
      "file:///C:/workspace/src/Main.kt",
    );
  });

  it.each([
    [
      WINDOWS_TASK_ROOT_URI,
      BACKEND_REPOSITORY,
      KOTLIN_SOURCE_PATH,
      "file:///C:/Task%20Root/backend/src/Main.kt",
    ],
    [
      UNC_WORKSPACE_URI,
      BACKEND_REPOSITORY,
      KOTLIN_SOURCE_PATH,
      "file://build-server/work%20share/backend/src/Main.kt",
    ],
  ])("joins paths without losing drive or UNC roots", (root, repo, path, expected) => {
    expect(joinFileUri(root, repo, path)).toBe(expected);
  });

  it.each(["../outside.kt", "/outside.kt", "repo/../../outside.kt"])(
    "rejects non-workspace-relative document path %s",
    (path) => {
      expect(() => joinFileUri(POSIX_WORKSPACE_URI, undefined, path)).toThrow(/workspace-relative/);
    },
  );

  it("rejects drive-absolute children under a Windows workspace", () => {
    expect(() => joinFileUri(WINDOWS_WORKSPACE_URI, undefined, "C:\\outside.kt")).toThrow(
      /workspace-relative/,
    );
  });

  it("canonicalizes equivalent encoded file URIs", () => {
    expect(canonicalFileUri("file:///workspace/A%23B%3f.kt")).toBe("file:///workspace/A%23B%3F.kt");
    expect(canonicalFileUri("file:///task:root/A!B(C):D.kt")).toBe(
      "file:///task%3Aroot/A%21B%28C%29%3AD.kt",
    );
    expect(canonicalFileUri("https://example.com/A.kt")).toBeNull();
  });
});

describe("LSP file URI workspace identity", () => {
  it("resolves encoded definition URIs into repository-scoped editor locations", () => {
    expect(
      resolveFileUriInWorkspace(
        "file:///task%20root/backend/src/My%20Definition.kt",
        ENCODED_WORKSPACE_URI,
        ["front", BACKEND_REPOSITORY],
      ),
    ).toEqual({ repo: BACKEND_REPOSITORY, path: "src/My Definition.kt" });
  });

  it("keeps root-relative files root-relative when no repository matches", () => {
    expect(
      resolveFileUriInWorkspace("file:///task%20root/docs/Guide%20%231.md", ENCODED_WORKSPACE_URI, [
        BACKEND_REPOSITORY,
      ]),
    ).toEqual({ path: "docs/Guide #1.md" });
  });

  it.each([
    "file:///workspace-other/Main.kt",
    POSIX_WORKSPACE_URI,
    "file://other-host/workspace/Main.kt",
  ])("rejects URI outside the workspace boundary: %s", (uri) => {
    expect(resolveFileUriInWorkspace(uri, POSIX_WORKSPACE_URI, [BACKEND_REPOSITORY])).toBeNull();
  });

  it("resolves Windows and UNC document URIs using task-host semantics", () => {
    expect(
      resolveFileUriInWorkspace(
        "file:///c:/Task%20Root/backend/src/Main.kt",
        WINDOWS_TASK_ROOT_URI,
        [BACKEND_REPOSITORY],
      ),
    ).toEqual({ repo: BACKEND_REPOSITORY, path: KOTLIN_SOURCE_PATH });
    expect(
      resolveFileUriInWorkspace(
        "file:///C:/Task%20Root/BACKEND/src/Main.kt",
        WINDOWS_TASK_ROOT_URI,
        [BACKEND_REPOSITORY],
      ),
    ).toEqual({ repo: BACKEND_REPOSITORY, path: KOTLIN_SOURCE_PATH });
    expect(
      resolveFileUriInWorkspace(
        "file://BUILD-SERVER/work%20share/backend/src/Main.kt",
        UNC_WORKSPACE_URI,
        [BACKEND_REPOSITORY],
      ),
    ).toEqual({ repo: BACKEND_REPOSITORY, path: KOTLIN_SOURCE_PATH });
  });

  it("compares Windows file URIs case-insensitively without weakening POSIX identity", () => {
    expect(
      fileUrisEqual("file:///C:/TaskRoot/src/Main.kt", "file:///c:/taskroot/SRC/main.kt"),
    ).toBe(true);
    expect(
      fileUrisEqual("file://BUILD-SERVER/Work/src/Main.kt", "file://build-server/work/SRC/main.kt"),
    ).toBe(true);
    expect(fileUrisEqual("file:///workspace/Main.kt", "file:///workspace/main.kt")).toBe(false);
  });

  it.each([
    "file:///workspace/src/Main%20%23.kt",
    `${WINDOWS_TASK_ROOT_URI}/src/Main.ts`,
    `${UNC_WORKSPACE_URI}/src/Main.tsx`,
    "file:///task%3Aroot/src/A%21B%28C%29%3AD.kt",
  ])("round-trips session-qualified Monaco model identity for %s", (documentUri) => {
    const firstModel = modelUriForDocument(documentUri, "session/A");
    const secondModel = modelUriForDocument(documentUri, "session/B");

    expect(firstModel).not.toBe(secondModel);
    expect(firstModel).toMatch(/\.(?:kt|ts|tsx)$/);
    expect(firstModel).not.toMatch(/[?#]/);
    expect(isSessionModelUri(firstModel)).toBe(true);
    const monacoRoundTrip = MonacoUri.parse(firstModel).toString();
    expect(monacoRoundTrip).toBe(firstModel);
    expect(documentUriForModel(monacoRoundTrip, "session/A")).toBe(canonicalFileUri(documentUri));
    expect(documentUriForModel(firstModel, "session/B")).toBeNull();
    expect(documentUriForModel(documentUri, "session/A")).toBeNull();
    expect(documentUriForModel(`${firstModel}?unexpected=1`, "session/A")).toBeNull();
  });

  it("rejects encoded Windows separators that would traverse the workspace", () => {
    expect(
      resolveFileUriInWorkspace(
        "file:///C:/Task%20Root/%5C..%5Csecret.kt",
        WINDOWS_TASK_ROOT_URI,
        [],
      ),
    ).toBeNull();
  });
});
