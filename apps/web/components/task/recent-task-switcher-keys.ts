import { MODIFIER_KEYS, type KeyboardShortcut, type Platform } from "@/lib/keyboard/constants";
import { detectPlatform, matchesShortcut } from "@/lib/keyboard/utils";

type HoldModifier = (typeof MODIFIER_KEYS)[keyof typeof MODIFIER_KEYS];

function resolvePlatform(platform?: Platform): Platform {
  return platform ?? detectPlatform();
}

function getCtrlOrCmdModifier(platform?: Platform): HoldModifier {
  return resolvePlatform(platform) === "mac" ? MODIFIER_KEYS.CMD : MODIFIER_KEYS.CTRL;
}

export function getHoldModifier(
  shortcut: KeyboardShortcut,
  platform?: Platform,
): HoldModifier | null {
  const modifiers = shortcut.modifiers;
  if (!modifiers) return null;

  if (modifiers.ctrlOrCmd) return getCtrlOrCmdModifier(platform);
  if (modifiers.cmd) return MODIFIER_KEYS.CMD;
  if (modifiers.ctrl) return MODIFIER_KEYS.CTRL;
  if (modifiers.alt) return MODIFIER_KEYS.ALT;
  if (modifiers.shift) return MODIFIER_KEYS.SHIFT;
  return null;
}

export function hasHoldModifier(shortcut: KeyboardShortcut): boolean {
  return getHoldModifier(shortcut) !== null;
}

export function isCycleShortcutEvent(event: KeyboardEvent, shortcut: KeyboardShortcut): boolean {
  if (event.repeat) return false;
  return matchesShortcut(event, shortcut);
}

export function isCommitReleaseEvent(
  event: KeyboardEvent,
  shortcut: KeyboardShortcut,
  platform?: Platform,
): boolean {
  return event.key === getHoldModifier(shortcut, platform);
}
