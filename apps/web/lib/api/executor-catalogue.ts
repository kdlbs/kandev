let epoch = 0;
const obsoleteRead = Symbol();

export function executorCatalogueEpoch() {
  return epoch;
}

export function invalidateExecutorCatalogue() {
  return ++epoch;
}

export async function readExecutorCatalogue<T>(read: () => Promise<T>): Promise<T> {
  const observed = epoch;
  const value = await read();
  if (observed !== epoch) throw obsoleteRead;
  return value;
}
