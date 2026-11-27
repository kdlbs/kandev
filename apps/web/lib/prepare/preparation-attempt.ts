type ParsedTimestamp = { seconds: number; nanoseconds: number };

function parseRfc3339Nano(value: string): ParsedTimestamp | null {
  const match =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(
      value,
    );
  if (!match) return null;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, fraction = "", zone] =
    match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const hour = Number(hourText);
  const minute = Number(minuteText);
  const second = Number(secondText);
  if (month < 1 || month > 12 || hour > 23 || minute > 59 || second > 59) return null;

  const localDate = new Date(0);
  localDate.setUTCFullYear(year, month - 1, day);
  localDate.setUTCHours(hour, minute, second, 0);
  if (
    localDate.getUTCFullYear() !== year ||
    localDate.getUTCMonth() !== month - 1 ||
    localDate.getUTCDate() !== day
  ) {
    return null;
  }

  let offsetMinutes = 0;
  if (zone !== "Z") {
    const sign = zone[0] === "+" ? 1 : -1;
    const offsetHours = Number(zone.slice(1, 3));
    const offsetRemainder = Number(zone.slice(4, 6));
    if (offsetHours > 23 || offsetRemainder > 59) return null;
    offsetMinutes = sign * (offsetHours * 60 + offsetRemainder);
  }

  return {
    seconds: localDate.getTime() / 1000 - offsetMinutes * 60,
    nanoseconds: Number(fraction.padEnd(9, "0")),
  };
}

/** Exact ordering for RFC3339Nano preparation attempt timestamps. */
export function comparePreparationStartedAt(left: string, right: string): -1 | 0 | 1 | null {
  const parsedLeft = parseRfc3339Nano(left);
  const parsedRight = parseRfc3339Nano(right);
  if (!parsedLeft || !parsedRight) return null;
  if (parsedLeft.seconds < parsedRight.seconds) return -1;
  if (parsedLeft.seconds > parsedRight.seconds) return 1;
  if (parsedLeft.nanoseconds < parsedRight.nanoseconds) return -1;
  if (parsedLeft.nanoseconds > parsedRight.nanoseconds) return 1;
  return 0;
}
