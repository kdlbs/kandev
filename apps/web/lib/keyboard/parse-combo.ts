/**
 * Parses a plugin manifest keybinding combo string (e.g. "mod+shift+k") into
 * the core `KeyboardShortcut` shape consumed by `matchesShortcut`.
 *
 * Grammar mirrors the backend validator
 * (`apps/backend/internal/plugins/manifest/validate.go` `parseKeybindingCombo`),
 * which already rejected malformed combos at plugin registration time:
 * `+`-separated tokens, modifiers from a fixed vocabulary, and exactly one
 * non-modifier key token. This parser is defensive (returns `null` instead of
 * throwing) since it also runs against values that already passed backend
 * validation.
 *
 * The accepted modifier vocabulary — `mod|ctrl|cmd|meta|alt|option|shift` —
 * matches the public contract in `docs/plans/plugins/PLUGIN-API.md` exactly.
 * "control"/"super" are deliberately NOT accepted (kept in lockstep with the
 * backend validator, which also rejects them).
 *
 * `mod` maps to `ctrlOrCmd` in the `KeyboardShortcut` shape — `matchesShortcut`
 * already resolves `ctrlOrCmd` to Cmd on macOS / Ctrl elsewhere, so no
 * separate platform branch is needed here.
 *
 * Combos pairing `shift` with a key whose printable character Shift would
 * alter (digits, symbols — see `SHIFT_ALTERED_KEY_TOKENS`) are rejected: the
 * combo is matched against `KeyboardEvent.key`, but a physically-held Shift
 * means the browser reports the shifted character ("!", "?") instead of the
 * unshifted token, so the combo could never actually dispatch. This mirrors
 * the backend validator's `shiftAlteredKeys` rejection.
 */
import type { Key, KeyboardShortcut } from "./constants";

type ModifierToken = "mod" | "ctrl" | "cmd" | "meta" | "alt" | "option" | "shift";

const MODIFIER_TOKENS: Record<ModifierToken, keyof NonNullable<KeyboardShortcut["modifiers"]>> = {
  mod: "ctrlOrCmd",
  ctrl: "ctrl",
  cmd: "cmd",
  meta: "cmd",
  alt: "alt",
  option: "alt",
  shift: "shift",
};

/**
 * Non-modifier key tokens whose reported `KeyboardEvent.key` changes when
 * Shift is held (e.g. "1" -> "!", "slash" -> "?"). Must stay in sync with the
 * backend's `shiftAlteredKeys` in
 * `apps/backend/internal/plugins/manifest/validate.go`.
 */
const SHIFT_ALTERED_KEY_TOKENS = new Set<string>([
  "comma",
  "period",
  "slash",
  "backslash",
  "semicolon",
  "quote",
  "minus",
  "equal",
  "bracketleft",
  "bracketright",
  "backquote",
  "0",
  "1",
  "2",
  "3",
  "4",
  "5",
  "6",
  "7",
  "8",
  "9",
]);

/** Named-key tokens mapped to their `KeyboardEvent.key` equivalent. */
const NAMED_KEY_TOKENS: Record<string, string> = {
  enter: "Enter",
  escape: "Escape",
  esc: "Escape",
  tab: "Tab",
  space: " ",
  backspace: "Backspace",
  delete: "Delete",
  insert: "Insert",
  arrowup: "ArrowUp",
  arrowdown: "ArrowDown",
  arrowleft: "ArrowLeft",
  arrowright: "ArrowRight",
  up: "ArrowUp",
  down: "ArrowDown",
  left: "ArrowLeft",
  right: "ArrowRight",
  home: "Home",
  end: "End",
  pageup: "PageUp",
  pagedown: "PageDown",
  comma: ",",
  period: ".",
  slash: "/",
  backslash: "\\",
  semicolon: ";",
  quote: "'",
  minus: "-",
  equal: "=",
  bracketleft: "[",
  bracketright: "]",
  backquote: "`",
};

function isSingleAlphanumeric(token: string): boolean {
  return /^[a-z0-9]$/.test(token);
}

function isFunctionKey(token: string): boolean {
  return /^f([1-9]|1[0-2])$/.test(token);
}

/** Resolves a single non-modifier token to a `KeyboardShortcut["key"]`, or null if unrecognized. */
function resolveKeyToken(token: string): Key | null {
  if (token in NAMED_KEY_TOKENS) return NAMED_KEY_TOKENS[token] as Key;
  if (isSingleAlphanumeric(token) || isFunctionKey(token)) return token as Key;
  return null;
}

/**
 * Parses a combo string like "mod+shift+k" into a `KeyboardShortcut`.
 * Returns null when the combo is empty, contains an unknown token, does not
 * contain exactly one non-modifier key token, or pairs `shift` with a key
 * whose reported character shift would alter (see `SHIFT_ALTERED_KEY_TOKENS`).
 */
export function parseCombo(combo: string): KeyboardShortcut | null {
  if (!combo || !combo.trim()) return null;

  const tokens = combo.split("+").map((token) => token.trim().toLowerCase());
  const modifiers: NonNullable<KeyboardShortcut["modifiers"]> = {};
  let key: Key | null = null;
  let keyToken: string | null = null;

  for (const token of tokens) {
    if (!token) return null;

    if (token in MODIFIER_TOKENS) {
      modifiers[MODIFIER_TOKENS[token as ModifierToken]] = true;
      continue;
    }

    const resolvedKey = resolveKeyToken(token);
    if (!resolvedKey) return null;
    if (key !== null) return null; // more than one non-modifier key token
    key = resolvedKey;
    keyToken = token;
  }

  if (key === null || keyToken === null) return null;
  if (modifiers.shift && SHIFT_ALTERED_KEY_TOKENS.has(keyToken)) return null;

  return Object.keys(modifiers).length > 0 ? { key, modifiers } : { key };
}
