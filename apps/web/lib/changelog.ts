import changelogData from "@/generated/changelog.json";

export type ChangelogEntry = {
  version: string;
  date: string;
  notes: string;
};

export function getChangelog(): ChangelogEntry[] {
  return changelogData;
}
