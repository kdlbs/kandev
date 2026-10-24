import type {
  SessionCapacitySettingsValue,
  SessionCapacitySettingsPatch,
} from "@/lib/types/system";

export type GradeLimitDraft = { id: string; grade: string; maximum: string };

export function gradeLimitDrafts(limits: Record<string, number> = {}): GradeLimitDraft[] {
  return Object.entries(limits).map(([grade, maximum]) => ({
    id: `saved:${grade}`,
    grade,
    maximum: String(maximum),
  }));
}

export function gradeDraftSignature(rows: GradeLimitDraft[]): string {
  return JSON.stringify(rows.map(({ grade, maximum }) => [grade, maximum]));
}

function validGradeName(grade: string): boolean {
  const characters = Array.from(grade);
  return (
    grade !== "" &&
    grade.trim() === grade &&
    characters.length <= 64 &&
    !characters.some((character) => {
      const code = character.codePointAt(0)!;
      return code < 32 || (code >= 127 && code <= 159);
    })
  );
}

function parseGradeLimits(rows: GradeLimitDraft[]): Record<string, number> | null {
  const entries: [string, number][] = [];
  const seen = new Set<string>();
  for (const row of rows) {
    const maximum = row.maximum.trim();
    const parsed = Number(maximum);
    if (
      !validGradeName(row.grade) ||
      seen.has(row.grade) ||
      !/^\d+$/.test(maximum) ||
      !Number.isSafeInteger(parsed) ||
      parsed > 2147483647
    )
      return null;
    seen.add(row.grade);
    entries.push([row.grade, parsed]);
  }
  return Object.fromEntries(entries);
}

export function capacityGradePatch(
  saved: SessionCapacitySettingsValue,
  enabled: boolean,
  policy: string,
  rows: GradeLimitDraft[],
): SessionCapacitySettingsPatch | null {
  // Disabling preserves the saved grade book. Legacy backends receive total-only patches.
  if (!enabled || saved.policy === undefined) return {};
  if (policy !== "automatic" && policy !== "all") return null;
  const grades = parseGradeLimits(rows);
  if (
    grades === null ||
    (policy === "all" && rows.length === 0) ||
    (policy === "automatic" && rows.length > 0)
  )
    return null;
  const patch: SessionCapacitySettingsPatch = {};
  if (policy !== saved.policy) patch.policy = policy;
  const previous = saved.grade_limits ?? {};
  if (
    Object.keys(previous).length !== rows.length ||
    Object.entries(grades).some(
      ([grade, maximum]) => !Object.hasOwn(previous, grade) || previous[grade] !== maximum,
    )
  )
    patch.grade_limits = grades;
  return patch;
}
