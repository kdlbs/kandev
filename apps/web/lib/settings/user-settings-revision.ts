const REVISION_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/;

function parseUserSettingsRevision(
  value: string | null | undefined,
): readonly [epochMilliseconds: number, nanoseconds: number] | null {
  if (!value) return null;
  const match = REVISION_PATTERN.exec(value);
  if (!match) return null;
  const [
    ,
    year,
    month,
    day,
    hour,
    minute,
    second,
    fraction = "",
    zone,
    sign,
    offsetHour,
    offsetMinute,
  ] = match;
  const instant = new Date(0);
  instant.setUTCFullYear(Number(year), Number(month) - 1, Number(day));
  instant.setUTCHours(Number(hour), Number(minute), Number(second), 0);
  if (Number.isNaN(instant.getTime())) return null;
  const offset =
    zone === "Z" ? 0 : (Number(offsetHour) * 60 + Number(offsetMinute)) * (sign === "+" ? 1 : -1);
  const epochMilliseconds = instant.getTime() - offset * 60_000;
  return [epochMilliseconds, Number(fraction.padEnd(9, "0"))];
}

export function compareUserSettingsRevisions(
  left: string | null | undefined,
  right: string | null | undefined,
): -1 | 0 | 1 | null {
  if (left === right && left) return 0;
  const leftValue = parseUserSettingsRevision(left);
  const rightValue = parseUserSettingsRevision(right);
  if (leftValue === null && rightValue === null) return null;
  if (leftValue === null) return -1;
  if (rightValue === null) return 1;
  if (leftValue[0] !== rightValue[0]) return leftValue[0] > rightValue[0] ? 1 : -1;
  if (leftValue[1] === rightValue[1]) return 0;
  return leftValue[1] > rightValue[1] ? 1 : -1;
}
