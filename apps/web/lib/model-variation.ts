function isModelVariation(requested: string, advertised: string): boolean {
  const prefix = `${requested}[`;
  if (!advertised.startsWith(prefix) || !advertised.endsWith("]")) return false;

  const variant = advertised.slice(prefix.length, -1);
  return variant.length > 0 && !variant.includes("[") && !variant.includes("]");
}

export function findUniqueModelVariation(
  requested: string,
  advertised: readonly string[],
): string | null {
  if (!requested || requested.includes("[") || requested.includes("]")) return null;

  const candidates = new Set(advertised.filter((model) => isModelVariation(requested, model)));
  return candidates.size === 1 ? ([...candidates][0] ?? null) : null;
}
