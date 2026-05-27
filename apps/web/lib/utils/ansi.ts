// ANSI escape sequences. Covers:
//   - CSI/SGR colour + cursor codes: `\x1b[36m`, `\x1b[0m`, `\x1b[1m`, `\x1b[2K`, …
//   - OSC sequences (hyperlinks, window title) terminated by ST or BEL:
//     `\x1b]8;;url\x1b\\` or `\x1b]0;title\x07`
// Both render as literal glyph noise in a plain `<pre>`. npm, pnpm, yarn, and
// some make targets routinely emit the OSC variant for clickable hyperlinks.
const ANSI_RE = /\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\))/g;

export function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, "");
}
