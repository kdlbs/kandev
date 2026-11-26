import type { CLIFlag } from "@/lib/types/http";

export function curatedFlagEnabled(
  flags: CLIFlag[],
  flag: string,
  defaultEnabled: boolean,
): boolean {
  const matches = flags.filter((entry) => entry.flag === flag);
  return matches.length > 0 ? matches.some((entry) => entry.enabled) : defaultEnabled;
}

export function setCuratedFlagEnabled(
  flags: CLIFlag[],
  flag: string,
  description: string,
  enabled: boolean,
): CLIFlag[] {
  const matches = flags.filter((entry) => entry.flag === flag);
  const latest = matches.at(-1);
  const otherFlags = flags.filter((entry) => entry.flag !== flag);
  return [...otherFlags, { flag, description: latest?.description || description, enabled }];
}
