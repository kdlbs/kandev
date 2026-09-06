// Strict wire format: full RFC3339 with an explicit offset or UTC marker.
// Date.parse is permissive, so shape validation must happen before any date
// formatting or malformed values can be normalized into a different instant.
const RFC3339_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-](\d{2}):(\d{2}))$/;

const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

/** Whether the year is a leap year in the proleptic Gregorian calendar. */
function isLeapYear(year: number): boolean {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

/** Whether the day exists in the month, including leap-day validation. */
function validDayForMonth(year: number, month: number, day: number): boolean {
  if (day < 1) return false;
  const maxDay = month === 2 && isLeapYear(year) ? 29 : DAYS_IN_MONTH[month - 1];
  return day <= maxDay;
}

/** Returns the UTC offset in seconds for a matched RFC3339 zone. */
function parseOffsetSeconds(
  zone: string,
  offsetHourText: string | undefined,
  offsetMinuteText: string | undefined,
): number | null {
  if (zone === "Z") return 0;
  const hour = Number(offsetHourText);
  const minute = Number(offsetMinuteText);
  if (hour > 23 || minute > 59) return null;
  return (zone.startsWith("-") ? -1 : 1) * (hour * 60 + minute) * 60;
}

/**
 * Parses an RFC3339/RFC3339Nano timestamp into epoch nanoseconds.
 *
 * Missing, empty, malformed, and semantically invalid values return `null`.
 * Calendar and time components are validated explicitly because Date.parse
 * normalizes values such as February 30th instead of rejecting them.
 */
export function parseStrictRfc3339Timestamp(value: string | undefined): bigint | null {
  if (!value) return null;
  const match = RFC3339_PATTERN.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return null;
  if (!validDayForMonth(year, month, Number(match[3]))) return null;
  if (Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59) {
    return null;
  }
  const offsetSeconds = parseOffsetSeconds(match[8], match[9], match[10]);
  if (offsetSeconds === null) return null;
  // RFC3339Nano caps the fraction at 9 digits; longer fractions would
  // overflow into the seconds component when padded.
  if (match[7] !== undefined && match[7].length > 9) return null;

  // setUTCFullYear handles years 0-99 correctly; Date.UTC maps them to
  // 1900-1999 and would therefore normalize an otherwise valid timestamp.
  const utc = new Date(0);
  utc.setUTCFullYear(year, month - 1, Number(match[3]));
  utc.setUTCHours(Number(match[4]), Number(match[5]), Number(match[6]), 0);
  const wholeSeconds = BigInt(Math.floor(utc.getTime() / 1000)) - BigInt(offsetSeconds);
  const fractionNs = BigInt((match[7] ?? "").padEnd(9, "0"));
  return wholeSeconds * BigInt(1_000_000_000) + fractionNs;
}
