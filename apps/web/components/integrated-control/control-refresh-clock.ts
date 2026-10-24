const pad = (value: number) => String(value).padStart(2, "0");

/**
 * A fixed-width local clock for read times: `HH:MM:SS` today, `MM-DD HH:MM`
 * on another day, so the header never changes width between reads.
 */
export function formatControlClock(at: string | number, now: number = Date.now()): string {
  const date = new Date(at);
  if (!Number.isFinite(date.getTime())) return "";
  const today = new Date(now);
  const sameDay =
    date.getFullYear() === today.getFullYear() &&
    date.getMonth() === today.getMonth() &&
    date.getDate() === today.getDate();
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  if (!sameDay) return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${time}`;
  return `${time}:${pad(date.getSeconds())}`;
}
