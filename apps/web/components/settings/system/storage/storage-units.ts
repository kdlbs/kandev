const BYTES_PER_GIGABYTE = 1024 ** 3;

export function bytesToGigabytes(bytes: number): number {
  return bytes / BYTES_PER_GIGABYTE;
}

export function gigabytesToBytes(gigabytes: number): number {
  return Math.round(gigabytes * BYTES_PER_GIGABYTE);
}

export function formatGigabytes(bytes: number | null | undefined): string {
  if (bytes == null || !Number.isFinite(bytes)) return "-";
  if (bytes <= 0) return "0 GB";
  const gigabytes = bytesToGigabytes(bytes);
  if (gigabytes < 0.01) return "<0.01 GB";
  return `${Number(gigabytes.toFixed(2))} GB`;
}
