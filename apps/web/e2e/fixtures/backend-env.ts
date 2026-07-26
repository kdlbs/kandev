export class BackendFixtureEnvOverrides {
  private readonly entries = new Map<symbol, Record<string, string>>();

  add(overrides: Record<string, string>): () => void {
    this.assertNoConflicts(overrides);
    const token = Symbol("backend-fixture-env");
    this.entries.set(token, overrides);
    return () => this.entries.delete(token);
  }

  apply(
    baseline: Record<string, string>,
    oneShotOverrides?: Record<string, string>,
  ): Record<string, string> {
    if (oneShotOverrides) this.assertNoConflicts(oneShotOverrides);
    const env = { ...baseline };
    for (const overrides of this.entries.values()) Object.assign(env, overrides);
    if (oneShotOverrides) Object.assign(env, oneShotOverrides);
    return env;
  }

  private assertNoConflicts(next: Record<string, string>): void {
    for (const current of this.entries.values()) {
      for (const [key, value] of Object.entries(next)) {
        if (key in current && current[key] !== value) {
          throw new Error(`conflicting backend fixture environment override for ${key}`);
        }
      }
    }
  }
}

export function createScopedEnvUse(
  scopedEnv: BackendFixtureEnvOverrides,
  restart: () => Promise<void>,
): (overrides: Record<string, string>) => Promise<() => Promise<void>> {
  return async (overrides) => {
    const removeScope = scopedEnv.add(overrides);
    try {
      await restart();
    } catch (activationError) {
      removeScope();
      try {
        await restart();
      } catch (restoreError) {
        throw new AggregateError(
          [activationError, restoreError],
          "failed to activate backend fixture environment and restore baseline",
        );
      }
      throw activationError;
    }

    let released = false;
    return async () => {
      if (released) return;
      removeScope();
      await restart();
      released = true;
    };
  };
}
