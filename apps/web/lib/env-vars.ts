import type { ProfileEnvVar } from "@/lib/types/http";

/**
 * Deep-equality comparison for a profile's env_vars list. Mirrors
 * areCLIFlagsEqual in semantics: order matters, undefined fields normalize
 * to "" so a `{key, value: ""}` saved entry matches a freshly-loaded
 * `{key, value: undefined}` (the backend may omit empty strings on the wire).
 */
export function areEnvVarsEqual(
  a: ProfileEnvVar[] | null | undefined,
  b: ProfileEnvVar[] | null | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    if (
      left[i].key !== right[i].key ||
      (left[i].value ?? "") !== (right[i].value ?? "") ||
      (left[i].secret_id ?? "") !== (right[i].secret_id ?? "")
    ) {
      return false;
    }
  }
  return true;
}
