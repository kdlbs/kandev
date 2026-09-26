export type ParsedGitLabProjectUrl = {
  projectPath: string;
  branch?: string;
  path?: string;
};

// GitLab has no fixed host (self-managed instances are common) and workflow
// sync deliberately stores no host of its own — the workspace's configured
// GitLab connection supplies it at sync time. So unlike GitHub's parser this
// one cannot validate a hostname.
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

// stripGitSuffix removes a trailing ".git" from the final project-path
// segment, matching a pasted HTTPS clone URL (https://gitlab.com/g/p.git) —
// otherwise the ".git" becomes part of the stored project path and every
// sync 404s against a project that doesn't exist.
function stripGitSuffix(projectPath: string): string {
  return projectPath.replace(/\.git$/, "");
}

// parseGitLabProjectUrl extracts a project path (and, for /-/tree/... or
// /-/blob/... links, branch + directory) from a pasted GitLab link, an SSH
// remote, or a bare "group/subgroup/project" ref. Returns null when the
// input isn't a recognizable project reference.
//
// A branch name can itself contain slashes (e.g. "features/my-ticket"), so
// splitting a combined "project/-/tree/branch/path" string can't always
// place the boundary correctly — this parser assumes a single-segment
// branch, same limitation as parseGitHubRepoUrl. Callers should expose
// branch and directory as their own directly-editable fields rather than
// relying on this parse alone to be authoritative.
export function parseGitLabProjectUrl(input: string): ParsedGitLabProjectUrl | null {
  const raw = input.trim();
  if (!raw) return null;

  const ssh = raw.match(SSH_URL_RE);
  if (ssh) {
    const projectPath = stripGitSuffix(ssh[2].replace(/^\/+|\/+$/g, ""));
    return projectPath.includes("/") ? { projectPath } : null;
  }

  return parseNonSshRef(raw);
}

function parseNonSshRef(raw: string): ParsedGitLabProjectUrl | null {
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
    if (segments && segments.length >= 2 && looksLikeBareHost(segments[0])) {
      segments = segments.slice(1);
      // A bare host plus a single remaining segment is a malformed ref
      // ("gitlab.com/project"): after stripping the presumed host there is
      // no namespace left, so it cannot be a valid project path. The final
      // guard below catches it, but slicing first keeps the domain-looking
      // segment from being stored as a namespace.
      if (segments.length < 2) return null;
    }
  }
  if (!segments || segments.length < 2) return null;

  const markerIndex = segments.indexOf("-");
  if (markerIndex === -1) {
    return { projectPath: stripGitSuffix(segments.join("/")) };
  }
  const projectSegments = segments.slice(0, markerIndex);
  if (projectSegments.length < 2) return null;
  return {
    projectPath: stripGitSuffix(projectSegments.join("/")),
    ...parseBranchAndPath(segments.slice(markerIndex + 1)),
  };
}

function parseBranchAndPath(segments: string[]): Pick<ParsedGitLabProjectUrl, "branch" | "path"> {
  const [marker, branch, ...rest] = segments;
  if ((marker !== "tree" && marker !== "blob") || !branch) return {};
  const pathSegments = marker === "blob" ? rest.slice(0, -1) : rest;
  if (pathSegments.length === 0) return { branch };
  return { branch, path: pathSegments.join("/") };
}
