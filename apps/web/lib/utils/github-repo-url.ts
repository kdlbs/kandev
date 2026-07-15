export type ParsedGitHubRepoUrl = {
  owner: string;
  repo: string;
  branch?: string;
  path?: string;
};

const GITHUB_HOST = "github.com";
const SSH_URL_RE = /^git@github\.com:([^/\s]+)\/([^/\s]+?)(?:\.git)?\/?$/;

// parseGitHubRepoUrl extracts owner/repo (and branch + directory for
// /tree/... and /blob/... links) from a pasted GitHub URL. Returns null when
// the input isn't a recognizable GitHub repository link.
//
// Branch names containing "/" cannot be told apart from the leading path
// segments without asking the GitHub API, so single-segment branch names are
// assumed. A /blob/ link to a file resolves to the file's directory.
export function parseGitHubRepoUrl(input: string): ParsedGitHubRepoUrl | null {
  const raw = input.trim();
  if (!raw) return null;

  const ssh = raw.match(SSH_URL_RE);
  if (ssh) return { owner: ssh[1], repo: ssh[2] };

  let url: URL;
  try {
    url = new URL(raw.includes("://") ? raw : `https://${raw}`);
  } catch {
    return null;
  }
  if (url.hostname !== GITHUB_HOST && url.hostname !== `www.${GITHUB_HOST}`) return null;

  const segments = url.pathname.split("/").filter(Boolean).map(decodeURIComponent);
  if (segments.length < 2) return null;
  const [owner, rawRepo, ...rest] = segments;
  const repo = rawRepo.replace(/\.git$/, "");
  if (!owner || !repo) return null;
  return { owner, repo, ...parseBranchAndPath(rest) };
}

function parseBranchAndPath(segments: string[]): Pick<ParsedGitHubRepoUrl, "branch" | "path"> {
  const [marker, branch, ...rest] = segments;
  if ((marker !== "tree" && marker !== "blob") || !branch) return {};
  const pathSegments = marker === "blob" ? rest.slice(0, -1) : rest;
  if (pathSegments.length === 0) return { branch };
  return { branch, path: pathSegments.join("/") };
}
