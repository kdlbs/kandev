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
import { SettingsFloatingSave, type SettingsSaveStatus } from "./settings-floating-save";

export type SettingsSaveRevision = string | number;

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
  discard: () => Promise<void> | void;
};

type RegisteredContributor = {
  contributor: SettingsSaveContributor;
  registrationOrder: number;
};

type Registry = {
  upsert: (contributor: SettingsSaveContributor) => void;
  unregister: (id: string) => void;
};

type SaveResult = {
  canLeave: boolean;
  failedIds: Set<string>;
};

const SettingsSaveRegistryContext = createContext<Registry | null>(null);

export function SettingsSaveProvider({ children }: { children: ReactNode }) {
  const { contributors, registry, dirtyContributors, refreshRegistry } = useContributorRegistry();
  const { status, saveAll, clearSavedStatus, markError } = useSaveCoordinator(
    contributors,
    refreshRegistry,
  );
  const pendingNavigationRef = useRef<NavigationIntent | null>(null);
  const discardingRef = useRef(false);
  const [pendingNavigation, setPendingNavigation] = useState<NavigationIntent | null>(null);
  const [isDiscarding, setIsDiscarding] = useState(false);
  const hasDirty = dirtyContributors.length > 0;
  const invalidReason = dirtyContributors.find(({ contributor }) => contributor.canSave === false)
    ?.contributor.invalidReason;
  const displayStatus = status === "saved" && hasDirty ? "dirty" : status;

  useEffect(() => {
    if (status !== "saved") return;
    const timeout = window.setTimeout(clearSavedStatus, 1500);
    return () => window.clearTimeout(timeout);
  }, [clearSavedStatus, status]);

  const handleSave = useCallback(async (): Promise<boolean> => {
    const result = await saveAll();
    if (!result.canLeave || !pendingNavigationRef.current) return result.canLeave;

    const intent = pendingNavigationRef.current;
    pendingNavigationRef.current = null;
    setPendingNavigation(null);
    intent.proceed();
    return true;
  }, [saveAll]);

  const discardAndLeave = useCallback(async () => {
    if (discardingRef.current) return;
    discardingRef.current = true;
    setIsDiscarding(true);
    try {
      for (const { contributor } of getDirtyContributors(contributors)) {
        await contributor.discard();
      }
    } catch {
      markError();
      return;
    } finally {
      discardingRef.current = false;
      setIsDiscarding(false);
    }
    const intent = pendingNavigationRef.current;
    pendingNavigationRef.current = null;
    setPendingNavigation(null);
    intent?.proceed();
  }, [contributors, markError]);

  const continueEditing = useCallback(() => {
    const intent = pendingNavigationRef.current;
    pendingNavigationRef.current = null;
    setPendingNavigation(null);
    intent?.cancel();
  }, []);

  useEffect(() => {
    if (!hasDirty) return;
    return setNavigationBlocker((intent) => {
      pendingNavigationRef.current?.cancel();
      pendingNavigationRef.current = intent;
      setPendingNavigation(intent);
    });
  }, [hasDirty]);

  useEffect(() => {
    if (!hasDirty) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [hasDirty]);

  return (
    <SettingsSaveRegistryContext.Provider value={registry}>
      {children}
      {(hasDirty || status === "saved") && (
        <SettingsFloatingSave
          status={displayStatus}
          dirtyContributorIds={dirtyContributors.map(({ contributor }) => contributor.id).join(",")}
          invalidReason={invalidReason}
          navigationIntent={pendingNavigation}
          isDiscarding={isDiscarding}
          onSave={handleSave}
          onDiscardAndLeave={discardAndLeave}
          onContinueEditing={continueEditing}
        />
      )}
    </SettingsSaveRegistryContext.Provider>
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
  const clearSavedStatus = useCallback(() => setStatus("dirty"), []);
  const markError = useCallback(() => setStatus("error"), []);
  const saveAll = useCallback(async (): Promise<SaveResult> => {
    if (savingRef.current) return { canLeave: false, failedIds: new Set() };
    const submitted = snapshotDirtyContributors(contributors);
    if (submitted.some(({ contributor }) => contributor.canSave === false)) {
      return { canLeave: false, failedIds: new Set() };
    }

    savingRef.current = true;
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
    setStatus(saveCompletionStatus(failedIds, hasNewerChanges));
    refreshRegistry();
    return { canLeave: failedIds.size === 0 && !hasNewerChanges, failedIds };
  }, [contributors, refreshRegistry]);

  return { status, saveAll, clearSavedStatus, markError };
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
  if (!registry) throw new Error("useSettingsSaveContributor requires SettingsSaveProvider");

  useLayoutEffect(() => {
    registry.upsert(contributor);
  });

  useEffect(() => {
    return () => registry.unregister(contributor.id);
  }, [contributor.id, registry]);
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
  return getDirtyContributors(contributors).map((entry) => ({
    ...entry,
    contributor: entry.contributor,
  }));
}

function hasNewerRevision(
  contributors: Map<string, RegisteredContributor>,
  submitted: SettingsSaveContributor,
): boolean {
  const current = contributors.get(submitted.id)?.contributor;
  return Boolean(current && !Object.is(current.revision, submitted.revision));
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
