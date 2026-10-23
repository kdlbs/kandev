import type { KeyboardPlatform } from "@vscode/markdown-editor";

/** Native history events use the same modifier contract as the pinned controller. */
export function getMarkdownKeyboardPlatform(userAgent = navigator.userAgent): KeyboardPlatform {
  if (userAgent.includes("Macintosh") || userAgent.includes("Mac OS X")) return "macos";
  if (userAgent.includes("Windows")) return "windows";
  return "linux";
}
