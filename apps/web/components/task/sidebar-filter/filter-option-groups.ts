export function buildOptionGroups<T extends { group?: string }>(
  options: T[],
): Array<{ heading: string; items: T[] }> {
  const buckets = new Map<string, T[]>();
  for (const opt of options) {
    const key = opt.group ?? "";
    const bucket = buckets.get(key);
    if (bucket) bucket.push(opt);
    else buckets.set(key, [opt]);
  }
  return [...buckets.entries()].map(([heading, items]) => ({ heading, items }));
}

export function hasGroupedOptions(options: Array<{ group?: string }>): boolean {
  return options.some((o) => o.group);
}
