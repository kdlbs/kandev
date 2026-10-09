import { copyToClipboard } from "./copy-to-clipboard";

const unsafeRepositoryPathControlCharacter = /[\u0000-\u001f\u007f]/;

export type CopyPathResult = "copied" | "unsafe" | "failed";

export async function copyPathToClipboard(path: string): Promise<CopyPathResult> {
  if (unsafeRepositoryPathControlCharacter.test(path)) return "unsafe";
  return (await copyToClipboard(path)) ? "copied" : "failed";
}
