import { execFileSync } from "node:child_process";

/** Advertise the provider identity while retaining the fixture's offline transport. */
export function configureGitHubOrigin(
  repositoryPath: string,
  githubURL: string,
  gitEnvironment: NodeJS.ProcessEnv,
): () => void {
  const git = (args: string[]) =>
    execFileSync("git", args, {
      cwd: repositoryPath,
      env: gitEnvironment,
      encoding: "utf8",
    }).trim();
  const originalOrigin = git(["config", "--get", "remote.origin.url"]);
  const rewriteKey = `url.${originalOrigin}.insteadOf`;
  git(["config", "--add", rewriteKey, githubURL]);
  git(["remote", "set-url", "origin", githubURL]);

  return () => {
    git(["remote", "set-url", "origin", originalOrigin]);
    git(["config", "--fixed-value", "--unset-all", rewriteKey, githubURL]);
  };
}
