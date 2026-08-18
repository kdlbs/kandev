"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { setNavigationBlocker, type NavigationIntent } from "@/lib/routing/navigation-guard";
import {
  SettingsFloatingSave,
  type SettingsSaveErrorKind,
  type SettingsSavePlacement,
  type SettingsSaveStatus,
} from "./settings-floating-save";

export type SettingsSaveRevision = string | number;

// i18n-exempt: control-flow signal. The save coordinator matches this error by
// type to abandon a save silently; the message is never rendered.
export class SettingsSaveCancelledError extends Error {
  constructor(message = "Save cancelled") {
    super(message);
    this.name = "SettingsSaveCancelledError";
  }
}

export type SettingsSaveContributor = {
  id: string;
  order?: number;
  revision: SettingsSaveRevision;
  isDirty: boolean;
  canSave?: boolean;
  invalidReason?: string;
  save: (revision: SettingsSaveRevision) => Promise<void> | void;
  discard: (revision?: SettingsSaveRevision) => Promise<void> | void;
};

type RegisteredContributor = {
  contributor: SettingsSaveContributor;
  registrationOrder: number;
};

type Registry = {
  upsert: (contributor: SettingsSaveContributor) => void;
  unregister: (id: string) => void;
};

type DirtyScopeRegistry = {
  upsert: (id: string, isDirty: boolean) => void;
  unregister: (id: string) => void;
};

type SaveResult = {
  canLeave: boolean;
  failedIds: Set<string>;
};

const SettingsSaveRegistryContext = createContext<Registry | null>(null);
const SettingsDirtyScopeContext = createContext<DirtyScopeRegistry | null>(null);

export type SettingsSaveCoordinator = {
  /** Save every dirty contributor. Respects each contributor's canSave;
   * returns canLeave=false when any contributor is invalid or fails. */
  saveAll: () => Promise<SaveResult>;
  status: SettingsSaveStatus;
  errorKind: SettingsSaveErrorKind | null;
  hasDirty: boolean;
  /** Reason why the first invalid dirty contributor cannot be saved, if any. */
  invalidReason: string | undefined;
  /** Cancel a navigation intent blocked by the dirty guard (the caller is
   * superseding it with its own navigation). */
  cancelPendingNavigation: () => void;
  clearSavedStatus: () => void;
};

const SettingsSaveCoordinatorContext = createContext<SettingsSaveCoordinator | null>(null);

export function SettingsSaveProvider({
  children,
  placement = "viewport",
}: {
  children: ReactNode;
  placement?: SettingsSavePlacement;
}) {
  const { contributors, registry, dirtyContributors, refreshRegistry } = useContributorRegistry();
  const { status, errorKind, saveAll, clearSavedStatus, markError } = useSaveCoordinator(
    contributors,
    refreshRegistry,
  );
  const hasDirty = dirtyContributors.length > 0;
  const invalidReason = dirtyContributors.find(({ contributor }) => contributor.canSave === false)
    ?.contributor.invalidReason;
  const displayStatus = status === "saved" && hasDirty ? "dirty" : status;

  useEffect(() => {
    if (status !== "saved") return;
    const timeout = window.setTimeout(clearSavedStatus, 1500);
    return () => window.clearTimeout(timeout);
  }, [clearSavedStatus, status]);

  const {
    pendingNavigation,
    isDiscarding,
    handleSave,
    discardDirtyContributors,
    discardAndLeave,
    continueEditing,
  } = useSaveNavigationFlow({ saveAll, contributors, markError, refreshRegistry, hasDirty });

  useSettingsBeforeUnloadGuard(hasDirty);

  return (
    <SettingsSaveProviderBody
      registry={registry}
      coordinator={{
        saveAll,
        status,
        errorKind,
        hasDirty,
        invalidReason,
        cancelPendingNavigation: continueEditing,
        clearSavedStatus,
      }}
      placement={placement}
      displayStatus={displayStatus}
      errorKind={errorKind}
      dirtyContributorIds={dirtyContributors.map(({ contributor }) => contributor.id).join(",")}
      invalidReason={invalidReason}
      pendingNavigation={pendingNavigation}
      isDiscarding={isDiscarding}
      onSave={handleSave}
      onReset={discardDirtyContributors}
      onDiscardAndLeave={discardAndLeave}
      onContinueEditing={continueEditing}
    >
      {children}
    </SettingsSaveProviderBody>
  );
}

type SettingsSaveProviderBodyProps = {
  registry: Registry;
  coordinator: SettingsSaveCoordinator;
  placement: SettingsSavePlacement;
  displayStatus: SettingsSaveStatus;
  errorKind: SettingsSaveErrorKind | null;
  dirtyContributorIds: string;
  invalidReason: string | undefined;
  pendingNavigation: NavigationIntent | null;
  isDiscarding: boolean;
  onSave: () => Promise<boolean>;
  onReset: () => Promise<boolean>;
  onDiscardAndLeave: () => Promise<void>;
  onContinueEditing: () => void;
  children: ReactNode;
};

/**
 * Renders the provider's context stack (registry + coordinator) around the
 * page content and the floating save bar/dialog. Extracted to keep
 * SettingsSaveProvider under the line limit.
 */
function SettingsSaveProviderBody({
  registry,
  coordinator,
  placement,
  displayStatus,
  errorKind,
  dirtyContributorIds,
  invalidReason,
  pendingNavigation,
  isDiscarding,
  onSave,
  onReset,
  onDiscardAndLeave,
  onContinueEditing,
  children,
}: SettingsSaveProviderBodyProps) {
  const hasDirty = coordinator.hasDirty;
  return (
    <SettingsSaveRegistryContext.Provider value={registry}>
      <SettingsSaveCoordinatorContext.Provider value={coordinator}>
        {children}
        {(hasDirty || displayStatus === "saved") && (
          <SettingsFloatingSave
            status={displayStatus}
            placement={placement}
            errorKind={errorKind}
            dirtyContributorIds={dirtyContributorIds}
            invalidReason={invalidReason}
            navigationIntent={pendingNavigation}
            isDiscarding={isDiscarding}
            onSave={onSave}
            onReset={onReset}
            onDiscardAndLeave={onDiscardAndLeave}
            onContinueEditing={onContinueEditing}
          />
        )}
      </SettingsSaveCoordinatorContext.Provider>
    </SettingsSaveRegistryContext.Provider>
  );
}

/**
 * Imperative handle on the shared settings save coordinator: save every dirty
 * contributor (across pages that register multiple contributors, e.g. the
 * profile editor and its MCP card) and learn whether the page can be left.
 * Requires SettingsSaveProvider.
 */
export function useSettingsSaveCoordinator(): SettingsSaveCoordinator {
  const coordinator = useContext(SettingsSaveCoordinatorContext);
  if (!coordinator) throw new Error("useSettingsSaveCoordinator requires SettingsSaveProvider");
  return coordinator;
}

/**
 * Owns the save/discard/continue actions and the dirty-navigation blocker:
 * registers the blocker while anything is dirty, saves and settles a pending
 * navigation intent, or discards the dirty contributors and leaves.
 */
function useSaveNavigationFlow({
  saveAll,
  contributors,
  markError,
  refreshRegistry,
  hasDirty,
}: {
  saveAll: () => Promise<SaveResult>;
  contributors: Map<string, RegisteredContributor>;
  markError: (kind: SettingsSaveErrorKind) => void;
  refreshRegistry: () => void;
  hasDirty: boolean;
}) {
  const pendingNavigationRef = useRef<NavigationIntent | null>(null);
  const discardingRef = useRef(false);
  const [pendingNavigation, setPendingNavigation] = useState<NavigationIntent | null>(null);
  const [isDiscarding, setIsDiscarding] = useState(false);

  useEffect(() => {
    if (!hasDirty) return;
    return setNavigationBlocker((intent) => {
      pendingNavigationRef.current?.cancel();
      pendingNavigationRef.current = intent;
      setPendingNavigation(intent);
    });
  }, [hasDirty]);

  const handleSave = useCallback(async (): Promise<boolean> => {
    const result = await saveAll();
    if (!result.canLeave) return false;
    settlePendingNavigation(pendingNavigationRef, setPendingNavigation);
    return true;
  }, [saveAll]);

  const discardDirtyContributors = useCallback(async (): Promise<boolean> => {
    if (discardingRef.current) return false;
    discardingRef.current = true;
    setIsDiscarding(true);
    const submitted = snapshotDirtyContributors(contributors);
    let hasNewerChanges = false;
    try {
      for (const { contributor } of submitted) {
        if (!isCurrentRevision(contributors, contributor)) {
          hasNewerChanges = true;
          continue;
        }
        await contributor.discard(contributor.revision);
        hasNewerChanges ||= hasNewerRevision(contributors, contributor);
      }
      refreshRegistry();
      if (hasNewerChanges) return false;
      settlePendingNavigation(pendingNavigationRef, setPendingNavigation);
      return true;
    } catch {
      markError("reset");
      return false;
    } finally {
      discardingRef.current = false;
      setIsDiscarding(false);
    }
  }, [contributors, markError, refreshRegistry]);

  const discardAndLeave = useCallback(async () => {
    await discardDirtyContributors();
  }, [discardDirtyContributors]);

  const continueEditing = useCallback(() => {
    const intent = pendingNavigationRef.current;
    pendingNavigationRef.current = null;
    setPendingNavigation(null);
    intent?.cancel();
  }, []);

  return {
    pendingNavigation,
    isDiscarding,
    handleSave,
    discardDirtyContributors,
    discardAndLeave,
    continueEditing,
  };
}

function useSettingsBeforeUnloadGuard(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [enabled]);
}

function settlePendingNavigation(
  pendingNavigationRef: { current: NavigationIntent | null },
  setPendingNavigation: (intent: NavigationIntent | null) => void,
) {
  const intent = pendingNavigationRef.current;
  pendingNavigationRef.current = null;
  setPendingNavigation(null);
  intent?.proceed();
}

export function SettingsSaveDirtyScope({
  children,
}: {
  children: (isDirty: boolean) => ReactNode;
}) {
  const dirtyIdsRef = useRef(new Set<string>());
  const [, setVersion] = useState(0);
  const registry = useMemo<DirtyScopeRegistry>(
    () => ({
      upsert: (id, isDirty) => {
        const changed = isDirty ? !dirtyIdsRef.current.has(id) : dirtyIdsRef.current.has(id);
        if (!changed) return;
        if (isDirty) dirtyIdsRef.current.add(id);
        else dirtyIdsRef.current.delete(id);
        setVersion((version) => version + 1);
      },
      unregister: (id) => {
        if (dirtyIdsRef.current.delete(id)) setVersion((version) => version + 1);
      },
    }),
    [],
  );

  return (
    <SettingsDirtyScopeContext.Provider value={registry}>
      {children(dirtyIdsRef.current.size > 0)}
    </SettingsDirtyScopeContext.Provider>
  );
}

function useContributorRegistry() {
  const contributorsRef = useRef(new Map<string, RegisteredContributor>());
  const registrationOrderRef = useRef(0);
  const [, setRegistryVersion] = useState(0);
  const refreshRegistry = useCallback(() => setRegistryVersion((version) => version + 1), []);
  const registry = useMemo<Registry>(
    () => ({
      upsert: (contributor) => {
        const existing = contributorsRef.current.get(contributor.id);
        if (!existing) {
          registrationOrderRef.current += 1;
          contributorsRef.current.set(contributor.id, {
            contributor,
            registrationOrder: registrationOrderRef.current,
          });
          refreshRegistry();
          return;
        }
        const changed = hasObservableChange(existing.contributor, contributor);
        existing.contributor = contributor;
        if (changed) refreshRegistry();
      },
      unregister: (id) => {
        if (contributorsRef.current.delete(id)) refreshRegistry();
      },
    }),
    [refreshRegistry],
  );

  return {
    contributors: contributorsRef.current,
    registry,
    dirtyContributors: getDirtyContributors(contributorsRef.current),
    refreshRegistry,
  };
}

function useSaveCoordinator(
  contributors: Map<string, RegisteredContributor>,
  refreshRegistry: () => void,
) {
  const savingRef = useRef(false);
  const [status, setStatus] = useState<SettingsSaveStatus>("dirty");
  const [errorKind, setErrorKind] = useState<SettingsSaveErrorKind | null>(null);
  const clearSavedStatus = useCallback(() => {
    setErrorKind(null);
    setStatus("dirty");
  }, []);
  const markError = useCallback((kind: SettingsSaveErrorKind) => {
    setErrorKind(kind);
    setStatus("error");
  }, []);
  const saveAll = useCallback(async (): Promise<SaveResult> => {
    if (savingRef.current) return { canLeave: false, failedIds: new Set() };
    const submitted = snapshotDirtyContributors(contributors);
    if (submitted.some(({ contributor }) => contributor.canSave === false)) {
      return { canLeave: false, failedIds: new Set() };
    }

    savingRef.current = true;
    setErrorKind(null);
    setStatus("saving");
    const failedIds = new Set<string>();
    let hasNewerChanges = false;
    for (const { contributor } of submitted) {
      try {
        await contributor.save(contributor.revision);
        hasNewerChanges ||= hasNewerRevision(contributors, contributor);
      } catch (error) {
        if (error instanceof SettingsSaveCancelledError) {
          hasNewerChanges = true;
          break;
        }
        failedIds.add(contributor.id);
      }
    }

    savingRef.current = false;
    // Revalidate the whole registry before reporting canLeave: a contributor
    // that became dirty while the saves were in flight (e.g. the MCP card
    // edited during the profile save) is absent from the submitted snapshot
    // and would otherwise be skipped while the caller proceeds on stale data.
    // Compare by id: the registry replaces the contributor object on every
    // upsert, so an identity check would report submitted contributors as
    // newly dirty after any re-render and block save-and-leave.
    const submittedIds = new Set(submitted.map(({ contributor }) => contributor.id));
    const newlyDirty = getDirtyContributors(contributors).filter(
      ({ contributor }) => !submittedIds.has(contributor.id),
    );
    const completionStatus = saveCompletionStatus(failedIds, hasNewerChanges);
    setErrorKind(completionStatus === "error" ? "save" : null);
    setStatus(completionStatus);
    refreshRegistry();
    return {
      canLeave: failedIds.size === 0 && !hasNewerChanges && newlyDirty.length === 0,
      failedIds,
    };
  }, [contributors, refreshRegistry]);

  return { status, errorKind, saveAll, clearSavedStatus, markError };
}

function saveCompletionStatus(
  failedIds: Set<string>,
  hasNewerChanges: boolean,
): SettingsSaveStatus {
  if (failedIds.size > 0) return "error";
  return hasNewerChanges ? "dirty" : "saved";
}

export function useSettingsSaveContributor(contributor: SettingsSaveContributor): void {
  const registry = useContext(SettingsSaveRegistryContext);
  const dirtyScope = useContext(SettingsDirtyScopeContext);
  if (!registry) throw new Error("useSettingsSaveContributor requires SettingsSaveProvider");

  useLayoutEffect(() => {
    registry.upsert(contributor);
    dirtyScope?.upsert(contributor.id, contributor.isDirty);
  });

  useEffect(() => {
    return () => {
      registry.unregister(contributor.id);
      dirtyScope?.unregister(contributor.id);
    };
  }, [contributor.id, dirtyScope, registry]);
}

function getDirtyContributors(
  contributors: Map<string, RegisteredContributor>,
): RegisteredContributor[] {
  return [...contributors.values()]
    .filter(({ contributor }) => contributor.isDirty)
    .sort(compareContributors);
}

function snapshotDirtyContributors(
  contributors: Map<string, RegisteredContributor>,
): RegisteredContributor[] {
  return getDirtyContributors(contributors).map((entry) => ({ ...entry }));
}

function hasNewerRevision(
  contributors: Map<string, RegisteredContributor>,
  submitted: SettingsSaveContributor,
): boolean {
  const current = contributors.get(submitted.id)?.contributor;
  return Boolean(current && current.isDirty && !Object.is(current.revision, submitted.revision));
}

function isCurrentRevision(
  contributors: Map<string, RegisteredContributor>,
  submitted: SettingsSaveContributor,
): boolean {
  const current = contributors.get(submitted.id)?.contributor;
  return Boolean(current && Object.is(current.revision, submitted.revision));
}

function compareContributors(left: RegisteredContributor, right: RegisteredContributor): number {
  const orderDifference =
    (left.contributor.order ?? Number.MAX_SAFE_INTEGER) -
    (right.contributor.order ?? Number.MAX_SAFE_INTEGER);
  return orderDifference || left.registrationOrder - right.registrationOrder;
}

function hasObservableChange(
  previous: SettingsSaveContributor,
  current: SettingsSaveContributor,
): boolean {
  return (
    previous.isDirty !== current.isDirty ||
    previous.canSave !== current.canSave ||
    previous.invalidReason !== current.invalidReason ||
    previous.order !== current.order ||
    !Object.is(previous.revision, current.revision)
  );
}
