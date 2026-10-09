export type DurationComponents = {
  value: number;
  minutes: boolean;
};

function ceilDiv(numerator: number, denominator: number): number {
  if (numerator <= 0) return 0;
  return Math.ceil(numerator / denominator);
}

/**
 * Mirrors apps/backend/internal/startup.EstimateComponents
 * (AC-PLATFORM-STARTUP-PROGRESS-003.10): 60000 ms or less rounds up to whole
 * seconds with a floor of one second while work remains; anything longer
 * rounds up to whole minutes. Every rendering surface that formats a
 * startup-progress duration (the Go page's embedded script and this web
 * dialog) must use the same rule so they never disagree on one snapshot.
 */
export function estimateDurationComponents(ms: number): DurationComponents {
  if (ms <= 60000) {
    const seconds = Math.max(1, ceilDiv(ms, 1000));
    return { value: seconds, minutes: false };
  }
  return { value: ceilDiv(ms, 60000), minutes: true };
}
