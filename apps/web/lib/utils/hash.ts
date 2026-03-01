/**
 * Simple djb2 hash — fast, non-cryptographic.
 * Used for change detection (e.g., detecting if file content changed).
 * Not suitable for security purposes.
 *
 * @param str - String to hash
 * @returns Hex-encoded hash string
 */
export function djb2Hash(str: string): string {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) + hash + str.charCodeAt(i)) | 0;
  }
  return (hash >>> 0).toString(16);
}
