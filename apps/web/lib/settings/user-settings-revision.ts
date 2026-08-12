function validRevision(value: number | null | undefined): number | null {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) return null;
  return value;
}

export function compareUserSettingsRevisions(
  left: number | null | undefined,
  right: number | null | undefined,
): -1 | 0 | 1 | null {
  const leftValue = validRevision(left);
  const rightValue = validRevision(right);
  if (leftValue === null && rightValue === null) return null;
  if (leftValue === null) return -1;
  if (rightValue === null) return 1;
  if (leftValue === rightValue) return 0;
  return leftValue > rightValue ? 1 : -1;
}
