import releaseNotesData from "@/generated/release-notes.json";

export type ReleaseNotes = {
  version: string;
  date: string;
  notes: string;
};

export function getReleaseNotes(): ReleaseNotes {
  return releaseNotesData;
}

export function hasReleaseNotes(): boolean {
  return !!releaseNotesData.notes && releaseNotesData.version !== "dev";
}

export function getReleaseUrl(version: string): string {
  return `https://github.com/kdlbs/kandev/releases/tag/v${version}`;
}
