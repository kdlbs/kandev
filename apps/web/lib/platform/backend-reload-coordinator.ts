export type BackendReloadSignal = "boot_id_changed" | "settings_interlock_rejected";

export type BackendReloadSnapshot = {
  reloadRequired: boolean;
  source: BackendReloadSignal | null;
  ownerCount: number;
};

export type BackendReloadDiagnosticReporter = (source: BackendReloadSignal) => void;

export type BackendReloadCoordinator = {
  getSnapshot: () => BackendReloadSnapshot;
  subscribe: (listener: () => void) => () => void;
  signal: (source: BackendReloadSignal) => boolean;
  registerOwner: () => () => void;
  setDiagnosticReporter: (reporter: BackendReloadDiagnosticReporter | null) => void;
};

const INITIAL_SNAPSHOT: BackendReloadSnapshot = {
  reloadRequired: false,
  source: null,
  ownerCount: 0,
};

export function createBackendReloadCoordinator(
  options: {
    reporter?: BackendReloadDiagnosticReporter;
  } = {},
): BackendReloadCoordinator {
  let snapshot = INITIAL_SNAPSHOT;
  let reporter = options.reporter ?? null;
  let reportSent = false;
  const listeners = new Set<() => void>();

  const notify = () => {
    listeners.forEach((listener) => listener());
  };

  const report = (source: BackendReloadSignal) => {
    if (reportSent || !reporter) return;
    reportSent = true;
    try {
      reporter(source);
    } catch {
      // Diagnostics must never change the recovery state.
    }
  };

  const signal = (source: BackendReloadSignal): boolean => {
    if (snapshot.reloadRequired) return false;
    snapshot = { reloadRequired: true, source, ownerCount: snapshot.ownerCount };
    notify();
    report(source);
    return true;
  };

  const registerOwner = (): (() => void) => {
    let released = false;
    snapshot = { ...snapshot, ownerCount: snapshot.ownerCount + 1 };
    notify();

    return () => {
      if (released) return;
      released = true;
      snapshot = { ...snapshot, ownerCount: Math.max(0, snapshot.ownerCount - 1) };
      notify();
    };
  };

  return {
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    signal,
    registerOwner,
    setDiagnosticReporter: (nextReporter) => {
      reporter = nextReporter;
      if (snapshot.reloadRequired && snapshot.source) report(snapshot.source);
    },
  };
}

export const backendReloadCoordinator = createBackendReloadCoordinator();

export const getBackendReloadSnapshot = () => backendReloadCoordinator.getSnapshot();

export const subscribeToBackendReload = (listener: () => void) =>
  backendReloadCoordinator.subscribe(listener);

export const signalBackendReloadRequired = (source: BackendReloadSignal) =>
  backendReloadCoordinator.signal(source);

export const registerBackendReloadOwner = () => backendReloadCoordinator.registerOwner();

export const setBackendReloadDiagnosticReporter = (
  reporter: BackendReloadDiagnosticReporter | null,
) => backendReloadCoordinator.setDiagnosticReporter(reporter);
