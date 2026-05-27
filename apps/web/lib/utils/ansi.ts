// ANSI escape sequences (CSI). Covers the SGR colour codes (`\x1b[36m`,
// `\x1b[0m`, `\x1b[1m`, …) that scripts and CLIs emit for terminal colour but
// which render as literal `[36m` noise when shown in a plain `<pre>`.
const ANSI_RE = /\x1b\[[0-9;]*[a-zA-Z]/g;

export function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, "");
}
