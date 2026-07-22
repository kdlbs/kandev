export type WorkspaceFileLocation = { path: string; repo?: string };

const WINDOWS_DRIVE_PATH = /^([A-Za-z]):[\\/](.*)$/;
const WINDOWS_DRIVE_URI_PATH = /^\/[A-Za-z]:\//;
const MONACO_MODEL_PREFIX = "/__kandev_session_model__/";

function uppercasePercentEscapes(value: string): string {
  return value.replace(/%[0-9a-f]{2}/gi, (escape) => escape.toUpperCase());
}

function encodePathSegments(path: string): string {
  return path.split("/").map(strictEncodeURIComponent).join("/");
}

function strictEncodeURIComponent(value: string): string {
  return encodeURIComponent(value).replace(
    /[!'()*]/g,
    (character) => `%${character.codePointAt(0)!.toString(16).toUpperCase()}`,
  );
}

function encodeIdentityToken(value: string): string {
  return [...new TextEncoder().encode(value)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

function decodeIdentityToken(value: string): string | null {
  if (!value || value.length % 2 !== 0 || !/^[0-9a-f]+$/i.test(value)) return null;
  const bytes = new Uint8Array(value.length / 2);
  for (let index = 0; index < value.length; index += 2) {
    bytes[index / 2] = Number.parseInt(value.slice(index, index + 2), 16);
  }
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

/** Convert an absolute task-host filesystem path into an RFC-safe file URI. */
export function filePathToUri(path: string): string {
  const drive = WINDOWS_DRIVE_PATH.exec(path);
  if (drive) {
    const tail = drive[2].replaceAll("\\", "/");
    return `file:///${drive[1]}:/${encodePathSegments(tail)}`;
  }

  if (path.startsWith("\\\\")) {
    const [host, ...segments] = path.slice(2).replaceAll("\\", "/").split("/");
    if (!host || segments.length === 0) throw new Error("UNC workspace path requires a share");
    return canonicalFileUri(`file://${host}/${encodePathSegments(segments.join("/"))}`)!;
  }

  if (!path.startsWith("/")) throw new Error("Workspace path must be absolute");
  return canonicalFileUri(`file:///${encodePathSegments(path.slice(1))}`)!;
}

/** Return a stable encoded file URI, or null for a non-file/ambiguous URI. */
export function canonicalFileUri(uri: string): string | null {
  try {
    const parsed = new URL(uri);
    if (parsed.protocol !== "file:" || parsed.search || parsed.hash) return null;
    const decodedSegments = parsed.pathname
      .split("/")
      .map((segment) => decodeURIComponent(segment));
    const encodedSegments = decodedSegments.map(strictEncodeURIComponent);
    if (!parsed.host && /^[A-Za-z]:$/.test(decodedSegments[1] ?? "")) {
      encodedSegments[1] = decodedSegments[1];
    }
    return uppercasePercentEscapes(`file://${parsed.host}${encodedSegments.join("/")}`);
  } catch {
    return null;
  }
}

function workspaceRelativeSegments(value: string, windowsSemantics: boolean): string[] {
  const normalized = windowsSemantics ? value.replaceAll("\\", "/") : value;
  if (normalized.startsWith("/") || (windowsSemantics && /^[A-Za-z]:\//.test(normalized))) {
    throw new Error("LSP document paths must be workspace-relative");
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.some((segment) => segment === "." || segment === "..")) {
    throw new Error("LSP document paths must be workspace-relative");
  }
  return segments;
}

/** Join a repo-relative editor path onto the task host's canonical workspace URI. */
export function joinFileUri(workspaceUri: string, repo: string | undefined, path: string): string {
  const canonicalWorkspace = canonicalFileUri(workspaceUri);
  if (!canonicalWorkspace) throw new Error("Invalid LSP workspace URI");

  const result = new URL(canonicalWorkspace);
  const decodedWorkspacePath = decodedUriPath(result);
  const windowsSemantics =
    Boolean(result.host) ||
    (decodedWorkspacePath !== null && WINDOWS_DRIVE_URI_PATH.test(decodedWorkspacePath));

  const segments = [
    ...(repo ? workspaceRelativeSegments(repo, windowsSemantics) : []),
    ...workspaceRelativeSegments(path, windowsSemantics),
  ];
  if (segments.length === 0) throw new Error("LSP document path is empty");

  const rootPath = result.pathname === "/" ? "" : result.pathname.replace(/\/+$/, "");
  result.pathname = `${rootPath}/${segments.map(strictEncodeURIComponent).join("/")}`;
  return canonicalFileUri(result.toString())!;
}

function decodedUriPath(uri: URL): string | null {
  try {
    return uri.pathname
      .split("/")
      .map((segment) => decodeURIComponent(segment))
      .join("/");
  } catch {
    return null;
  }
}

function fileUriIdentity(uri: string): string | null {
  const canonicalUri = canonicalFileUri(uri);
  if (!canonicalUri) return null;
  const parsed = new URL(canonicalUri);
  let path = decodedUriPath(parsed);
  if (path === null) return null;
  const windowsSemantics = WINDOWS_DRIVE_URI_PATH.test(path) || Boolean(parsed.host);
  if (windowsSemantics) path = path.replaceAll("\\", "/").toLocaleLowerCase("en-US");
  return `${parsed.host.toLocaleLowerCase("en-US")}\0${path}`;
}

/** Compare file URIs using task-host filesystem case and separator semantics. */
export function fileUrisEqual(left: string, right: string): boolean {
  const leftIdentity = fileUriIdentity(left);
  return leftIdentity !== null && leftIdentity === fileUriIdentity(right);
}

/** Build a browser-only Monaco model identity for one task session. */
export function modelUriForDocument(documentUri: string, sessionId: string): string {
  const canonicalDocument = canonicalFileUri(documentUri);
  if (!canonicalDocument || !sessionId) throw new Error("Invalid LSP document model identity");
  const document = new URL(canonicalDocument);
  const sessionToken = `s-${encodeIdentityToken(sessionId)}`;
  const hostToken = modelHostToken(document);
  const modelUri = `file://${MONACO_MODEL_PREFIX}${sessionToken}/${hostToken}${document.pathname}`;
  return canonicalFileUri(modelUri)!;
}

function modelHostToken(document: URL): string {
  if (document.host) return `h-${encodeIdentityToken(document.host)}`;
  return WINDOWS_DRIVE_URI_PATH.test(document.pathname) ? "d" : "l";
}

/** Strip and verify the session qualifier before sending a model URI to an LSP server. */
export function documentUriForModel(modelUri: string, sessionId: string): string | null {
  try {
    const parsed = new URL(modelUri);
    const identity = parseModelIdentity(parsed, sessionId);
    if (!identity) return null;
    return canonicalFileUri(`file://${identity.authority}/${identity.segments.join("/")}`);
  } catch {
    return null;
  }
}

type ModelIdentity = { authority: string; segments: string[] };

function parseModelIdentity(model: URL, sessionId: string): ModelIdentity | null {
  if (!hasModelUriEnvelope(model)) return null;
  const [sessionToken, hostToken, ...segments] = model.pathname
    .slice(MONACO_MODEL_PREFIX.length)
    .split("/");
  if (!sessionToken?.startsWith("s-") || !hostToken || !validModelSegments(segments)) return null;
  if (decodeIdentityToken(sessionToken.slice(2)) !== sessionId) return null;

  const authority = decodeModelAuthority(hostToken, segments);
  return authority === null ? null : { authority, segments };
}

function hasModelUriEnvelope(uri: URL): boolean {
  return (
    uri.protocol === "file:" &&
    !uri.host &&
    !uri.search &&
    !uri.hash &&
    uri.pathname.startsWith(MONACO_MODEL_PREFIX)
  );
}

function validModelSegments(segments: string[]): boolean {
  return segments.length > 0 && !segments.some((segment) => segment === "." || segment === "..");
}

function decodeModelAuthority(hostToken: string, segments: string[]): string | null {
  if (hostToken === "l") return "";
  if (hostToken !== "d") return decodeModelHost(hostToken);

  const drive = /^([A-Za-z])(?::|%3A)$/i.exec(segments[0]);
  if (!drive) return null;
  segments[0] = `${drive[1]}:`;
  return "";
}

function decodeModelHost(hostToken: string): string | null {
  if (!hostToken.startsWith("h-")) return null;
  const host = decodeIdentityToken(hostToken.slice(2));
  return host || null;
}

/** Whether a URI uses Kandev's browser-only, session-scoped Monaco identity. */
export function isSessionModelUri(uri: string): boolean {
  try {
    return hasModelUriEnvelope(new URL(uri));
  } catch {
    return false;
  }
}

function pathComparisonValue(path: string, windowsSemantics: boolean): string {
  return windowsSemantics ? path.toLocaleLowerCase("en-US") : path;
}

function normalizedRepoSegments(repo: string, windowsSemantics: boolean): string[] {
  const normalized = windowsSemantics ? repo.replaceAll("\\", "/") : repo;
  return normalized.split("/").filter(Boolean);
}

type RelativeUriPath = { segments: string[]; windowsSemantics: boolean };

function relativeUriPath(file: URL, workspace: URL): RelativeUriPath | null {
  if (file.host.toLocaleLowerCase("en-US") !== workspace.host.toLocaleLowerCase("en-US")) {
    return null;
  }

  let filePath = decodedUriPath(file);
  let workspacePath = decodedUriPath(workspace);
  if (!filePath || !workspacePath) return null;

  const windowsSemantics = WINDOWS_DRIVE_URI_PATH.test(workspacePath) || Boolean(workspace.host);
  if (windowsSemantics) {
    filePath = filePath.replaceAll("\\", "/");
    workspacePath = workspacePath.replaceAll("\\", "/");
  }
  workspacePath = workspacePath === "/" ? "/" : workspacePath.replace(/\/+$/, "");

  const comparableFile = pathComparisonValue(filePath, windowsSemantics);
  const comparableWorkspace = pathComparisonValue(workspacePath, windowsSemantics);
  const childPrefix = comparableWorkspace === "/" ? "/" : `${comparableWorkspace}/`;
  if (!comparableFile.startsWith(childPrefix) || comparableFile === comparableWorkspace)
    return null;

  const relative = filePath.slice(workspacePath === "/" ? 1 : workspacePath.length + 1);
  const segments = relative.split("/").filter(Boolean);
  if (segments.length === 0 || segments.some((segment) => segment === "." || segment === "..")) {
    return null;
  }
  return { segments, windowsSemantics };
}

function repositoryLocation(
  relative: RelativeUriPath,
  repositorySubpaths: Iterable<string>,
): WorkspaceFileLocation {
  const repositories = [...repositorySubpaths]
    .map((repo) => ({
      repo,
      segments: normalizedRepoSegments(repo, relative.windowsSemantics),
    }))
    .filter(({ segments }) => segments.length > 0)
    .sort((left, right) => right.segments.length - left.segments.length);
  for (const { repo, segments } of repositories) {
    const matches = segments.every((segment, index) => {
      const target = relative.segments[index];
      return relative.windowsSemantics
        ? segment.toLocaleLowerCase("en-US") === target?.toLocaleLowerCase("en-US")
        : segment === target;
    });
    if (segments.length < relative.segments.length && matches) {
      return { repo, path: relative.segments.slice(segments.length).join("/") };
    }
  }
  return { path: relative.segments.join("/") };
}

/**
 * Reverse a server file URI into the repo/path pair expected by workspace file APIs.
 * Returns null when the target is not a strict child of the task-host workspace URI.
 */
export function resolveFileUriInWorkspace(
  fileUri: string,
  workspaceUri: string,
  repositorySubpaths: Iterable<string> = [],
): WorkspaceFileLocation | null {
  const canonicalFile = canonicalFileUri(fileUri);
  const canonicalWorkspace = canonicalFileUri(workspaceUri);
  if (!canonicalFile || !canonicalWorkspace) return null;

  const file = new URL(canonicalFile);
  const workspace = new URL(canonicalWorkspace);
  const relative = relativeUriPath(file, workspace);
  return relative ? repositoryLocation(relative, repositorySubpaths) : null;
}
