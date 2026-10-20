import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

const shellQuote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

function directGitWhenUncontrolled(env: Record<string, string>): string {
  const originalPath = env.KANDEV_E2E_ORIGINAL_PATH;
  const delayFile = env.KANDEV_E2E_GIT_DELAY_FILE;
  const pushFile = env.KANDEV_E2E_GITLAB_PUSH_FILE;
  if (!originalPath || !delayFile || !pushFile) return "";
  // Controls can appear after worker startup, so check them on every command.
  // A command hint belongs to the delay control. Global Git options and push
  // controls keep the cross-platform Node parser as their owner.
  const exports = Object.entries(env).map(([key, value]) => shellQuote(`${key}=${value}`));
  return `git_uncontrolled=false
if [ ! -e ${shellQuote(pushFile)} ]; then
  if [ ! -e ${shellQuote(delayFile)} ]; then
    git_uncontrolled=true
  elif [ -f ${shellQuote(`${delayFile}.command`)} ]; then
    git_gate_command=
    IFS= read -r git_gate_command < ${shellQuote(`${delayFile}.command`)}
    case "$git_gate_command" in
      ""|*[!a-z-]*) ;;
      *)
        case "$1" in
          ""|-*) ;;
          *) [ "$1" = "$git_gate_command" ] || git_uncontrolled=true ;;
        esac
        ;;
    esac
  fi
fi
if [ "$git_uncontrolled" = true ]; then
  export ${exports.join(" ")}
  PATH=${shellQuote(originalPath)}
  export PATH
  exec git "$@"
fi
`;
}

export function writeGitShimLauncher(
  shimDir: string,
  shimScript: string,
  env: Record<string, string>,
): void {
  const node = process.execPath;
  const runner = path.join(shimDir, "git-runner.mjs");
  fs.writeFileSync(
    runner,
    `Object.assign(process.env, ${JSON.stringify(env)});\nawait import(${JSON.stringify(pathToFileURL(shimScript).href)});\n`,
  );
  if (process.platform === "win32") {
    // %* forwards all args verbatim; extensionless files aren't executable via
    // PATHEXT on Windows, so a .cmd wrapper is required for exec.Command("git").
    const launcher = `@echo off\r\n"${node}" "${runner}" %*\r\n`;
    fs.writeFileSync(path.join(shimDir, "git.cmd"), launcher);
    return;
  }
  // POSIX uses a non-login shell and directly execs ordinary Git commands.
  const launcher = `#!/bin/sh\n${directGitWhenUncontrolled(env)}exec ${shellQuote(node)} ${shellQuote(runner)} "$@"\n`;
  fs.writeFileSync(path.join(shimDir, "git"), launcher, { mode: 0o755 });
}
