import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

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
  // POSIX: an extensionless `git` shebang launcher. `#!/bin/sh` is only the
  // launcher interpreter (guaranteed present on macOS/Linux) — the shim body is
  // Node, so the developer's login shell (bash/zsh/fish) is irrelevant.
  const launcher = `#!/bin/sh\nexec "${node}" "${runner}" "$@"\n`;
  fs.writeFileSync(path.join(shimDir, "git"), launcher, { mode: 0o755 });
}
