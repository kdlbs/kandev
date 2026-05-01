/**
 * Transform a single input chunk based on active modifier state.
 *
 * - ctrl + single a-z/A-Z → the corresponding control byte (`\x01`..`\x1a`)
 * - shift + tab → CSI Z (reverse tab / backtab)
 *
 * Only single-character data is transformed — multi-char paste / IME output
 * passes through unchanged to avoid mangling dictated or autocompleted text.
 */
export function applyShellModifiers(data: string, mods: { ctrl: boolean; shift: boolean }): string {
  if (mods.ctrl && data.length === 1) {
    const code = data.charCodeAt(0);
    if (code >= 97 && code <= 122) return String.fromCharCode(code - 96);
    if (code >= 65 && code <= 90) return String.fromCharCode(code - 64);
  }
  if (mods.shift && data === "\t") return "\x1b[Z";
  return data;
}
