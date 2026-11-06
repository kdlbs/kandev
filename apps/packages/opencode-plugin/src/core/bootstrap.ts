import { readFile, stat } from "node:fs/promises";

import { ProtocolError, decodeBootstrap, type Bootstrap } from "../protocol.js";

/**
 * Reads the private launch bootstrap file. The credential lives only in this
 * file, never in plugin options or environment values.
 */
export async function readBootstrapFile(path: string): Promise<Bootstrap> {
  if (typeof path !== "string" || path.trim() === "") {
    throw new ProtocolError("bootstrap_missing", "plugin options do not name a bootstrap file");
  }
  let info;
  try {
    info = await stat(path);
  } catch {
    throw new ProtocolError("bootstrap_unreadable", "bootstrap file is unavailable");
  }
  if (!info.isFile()) {
    throw new ProtocolError("bootstrap_unreadable", "bootstrap path is not a regular file");
  }
  if (process.platform !== "win32" && (info.mode & 0o077) !== 0) {
    throw new ProtocolError(
      "bootstrap_permissions",
      "bootstrap file must not be readable by other users",
    );
  }
  let raw: string;
  try {
    raw = await readFile(path, "utf8");
  } catch {
    throw new ProtocolError("bootstrap_unreadable", "bootstrap file could not be read");
  }
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw new ProtocolError("bootstrap_invalid", "bootstrap file is not valid JSON");
  }
  return decodeBootstrap(value);
}

export function bootstrapOption(options: unknown): string {
  if (typeof options !== "object" || options === null || Array.isArray(options)) {
    throw new ProtocolError("bootstrap_missing", "plugin options are missing");
  }
  const value = (options as Record<string, unknown>).bootstrapFile;
  if (typeof value !== "string" || value.trim() === "") {
    throw new ProtocolError("bootstrap_missing", "plugin options do not name a bootstrap file");
  }
  return value;
}
