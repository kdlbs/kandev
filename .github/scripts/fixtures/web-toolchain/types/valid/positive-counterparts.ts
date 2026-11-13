/** Valid counterparts for the injected-error corpus: every shape that the
 * invalid project breaks must also have a shape that both compilers accept. */
export type Identified = { id: string; label: string };

export function rename(value: Identified, label: string): Identified {
  return { ...value, label };
}

export function pickId(value: Identified): string {
  return value.id;
}

export function sum(values: readonly number[]): number {
  return values.reduce((total, value) => total + value, 0);
}

export function formatCount(count: number): string {
  return `${count} items`;
}

export type BadgeTone = "neutral" | "danger" | "success";

export function badgeClass(tone: BadgeTone): string {
  switch (tone) {
    case "neutral":
      return "text-muted";
    case "danger":
      return "text-destructive";
    case "success":
      return "text-success";
  }
}
