import os from "node:os";

const LOCAL_WORKER_PERCENT = 20;

export function resolveMaxWorkers(
  value: string | undefined,
  isCI: boolean,
  allowUnsafe: boolean,
  availableParallelism = os.availableParallelism(),
): string | number | undefined {
  const normalized = value?.trim();
  if (isCI || allowUnsafe) return parseConfiguredWorkers(normalized);
  if (!normalized) return `${LOCAL_WORKER_PERCENT}%`;

  const percentage = parsePercentage(normalized);
  if (percentage !== null) {
    return percentage <= LOCAL_WORKER_PERCENT ? normalized : `${LOCAL_WORKER_PERCENT}%`;
  }

  const requestedWorkers = Number(normalized);
  if (Number.isInteger(requestedWorkers) && requestedWorkers > 0) {
    return Math.min(requestedWorkers, localWorkerBudget(availableParallelism));
  }
  return `${LOCAL_WORKER_PERCENT}%`;
}

function parseConfiguredWorkers(value: string | undefined): string | number | undefined {
  if (!value) return undefined;
  if (/^[1-9]\d*%$/.test(value)) return value;
  const workers = Number(value);
  return Number.isInteger(workers) && workers > 0 ? workers : undefined;
}

function parsePercentage(value: string): number | null {
  if (!/^[1-9]\d*%$/.test(value)) return null;
  return Number(value.slice(0, -1));
}

function localWorkerBudget(availableParallelism: number): number {
  return Math.max(1, Math.floor(availableParallelism * (LOCAL_WORKER_PERCENT / 100)));
}
