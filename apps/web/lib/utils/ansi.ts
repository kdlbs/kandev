// ANSI escape sequences. Covers:
//   - CSI sequences per ECMA-48: `ESC [ param* intermediate* final` where
//     param bytes are 0x30-0x3F (digits + `:;<=>?`), intermediate bytes are
//     0x20-0x2F (space + `!"#$%&'()*+,-./`), and final byte is 0x40-0x7E.
//     This catches both vanilla SGR (`\x1b[36m`, `\x1b[0m`) and private-prefix
//     sequences like `\x1b[?25l` (hide cursor) and `\x1b[?2004h` (bracketed
//     paste) that tools like pnpm and Playwright emit. The narrower
//     `[0-9;]*` pattern used to let those leak through and render as
//     literal `[?25l` noise.
//   - OSC sequences (hyperlinks, window title) terminated by ST or BEL:
//     `\x1b]8;;url\x1b\\` or `\x1b]0;title\x07`. npm, pnpm, yarn and some
//     make targets routinely emit the hyperlink variant.
const ANSI_RE = /\x1b(?:\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]|\][^\x07\x1b]*(?:\x07|\x1b\\))/g;

export function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, "");
}
