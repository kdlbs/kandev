export type ParsedGitLabProjectUrl = {
  projectPath: string;
  branch?: string;
  path?: string;
};

// GitLab has no fixed host (self-managed instances are common) and workflow
// sync deliberately stores no host of its own — the workspace's configured
// GitLab connection supplies it at sync time. So unlike GitHub's parser this
// one cannot validate a hostname, and round-tripping a saved config means
// re-parsing a host-less "project ref" (see buildGitLabProjectRef) rather
// than a real URL.
const SSH_URL_RE = /^git@([^:/\s]+):(.+?)(?:\.git)?\/?$/;

// A bare (schemeless) first segment that looks like a domain (contains a
// dot) is assumed to be a pasted address-bar URL missing its "https://" —
// common when copying from a browser — rather than a namespace segment.
// GitLab namespace paths can technically contain dots, so this is a
// heuristic, not a guarantee; a scheme-qualified URL is always unambiguous.
function looksLikeBareHost(segment: string): boolean {
  return segment.includes(".") && !segment.includes(" ");
}

function splitPath(pathname: string): string[] | null {
  try {
    return pathname.split("/").filter(Boolean).map(decodeURIComponent);
  } catch {
    // Malformed percent escapes must read as "not a recognized link".
    return null;
  }
}

// parseGitLabProjectUrl extracts a project path (and, for /-/tree/... or
// /-/blob/... links, branch + directory) from a pasted GitLab link, an SSH
// remote, or a bare "group/subgroup/project" ref. Returns null when the
// input isn't a recognizable project reference.
export function parseGitLabProjectUrl(input: string): ParsedGitLabProjectUrl | null {
  const raw = input.trim();
  if (!raw) return null;

  const ssh = raw.match(SSH_URL_RE);
  if (ssh) {
    const projectPath = ssh[2].replace(/^\/+|\/+$/g, "");
    return projectPath.includes("/") ? { projectPath } : null;
  }

  let segments: string[] | null;
  if (raw.includes("://")) {
    let url: URL;
    try {
      url = new URL(raw);
    } catch {
      return null;
    }
    segments = splitPath(url.pathname);
  } else {
    segments = splitPath(raw);
    if (segments && segments.length > 2 && looksLikeBareHost(segments[0])) {
      segments = segments.slice(1);
    }
  }
  if (!segments || segments.length < 2) return null;

  const markerIndex = segments.indexOf("-");
  if (markerIndex === -1) {
    return { projectPath: segments.join("/") };
  }
  const projectSegments = segments.slice(0, markerIndex);
  if (projectSegments.length < 2) return null;
  return { projectPath: projectSegments.join("/"), ...parseBranchAndPath(segments.slice(markerIndex + 1)) };
}

// buildGitLabProjectRef renders a stored config back into the host-less ref
// string parseGitLabProjectUrl accepts, so the settings form can redisplay a
// saved GitLab config without fabricating a host it was never given.
export function buildGitLabProjectRef(parts: ParsedGitLabProjectUrl): string {
  if (!parts.branch) return parts.projectPath;
  const path = parts.path ? `/${parts.path}` : "";
  return `${parts.projectPath}/-/tree/${parts.branch}${path}`;
}

function parseBranchAndPath(segments: string[]): Pick<ParsedGitLabProjectUrl, "branch" | "path"> {
  const [marker, branch, ...rest] = segments;
  if ((marker !== "tree" && marker !== "blob") || !branch) return {};
  const pathSegments = marker === "blob" ? rest.slice(0, -1) : rest;
  if (pathSegments.length === 0) return { branch };
  return { branch, path: pathSegments.join("/") };
}
