"use client";

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import type { AgentUpdateStatus } from "@/lib/api";
import { settingsActionClassName } from "./settings-control";
import { useRuntimeAutoUpdatePolicy } from "./use-runtime-auto-update-policy";

export type RuntimePolicyController = {
  key: string;
  agentName: string;
  status: AgentUpdateStatus;
  draft: boolean;
  isDirty: boolean;
  setDraft: (value: boolean) => void;
  reset: () => void;
};

type RuntimePolicyStore = {
  upsert: (controller: RuntimePolicyController) => void;
  remove: (key: string) => void;
  subscribe: (listener: () => void) => () => void;
  get: (agentName: string) => RuntimePolicyController | undefined;
  list: () => RuntimePolicyController[];
};

const EMPTY: RuntimePolicyController[] = [];
const RuntimePolicyContext = createContext<RuntimePolicyStore | null>(null);

function createRuntimePolicyStore(): RuntimePolicyStore {
  const byKey = new Map<string, RuntimePolicyController>();
  const listeners = new Set<() => void>();
  let snapshot: RuntimePolicyController[] = EMPTY;
  const publish = () => {
    snapshot = [...byKey.values()];
    for (const listener of listeners) listener();
  };
  const same = (left: RuntimePolicyController, right: RuntimePolicyController) =>
    left.status === right.status &&
    left.draft === right.draft &&
    left.isDirty === right.isDirty &&
    left.setDraft === right.setDraft &&
    left.reset === right.reset;
  return {
    upsert(controller) {
      const current = byKey.get(controller.key);
      if (current && same(current, controller)) return;
      byKey.set(controller.key, controller);
      publish();
    },
    remove(key) {
      if (byKey.delete(key)) publish();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    get(agentName) {
      return snapshot.find((controller) => controller.agentName === agentName);
    },
    list() {
      return snapshot;
    },
  };
}

function RuntimePolicyContributor({
  store,
  status,
}: {
  store: RuntimePolicyStore;
  status: AgentUpdateStatus;
}) {
  const { draft, setDraft, isDirty, reset } = useRuntimeAutoUpdatePolicy(status);
  const key = `${status.agent_name}:${status.runtime_id}`;
  const controller = useRef({
    key,
    agentName: status.agent_name,
    status,
    draft,
    isDirty,
    setDraft,
    reset,
  });
  controller.current = {
    key,
    agentName: status.agent_name,
    status,
    draft,
    isDirty,
    setDraft,
    reset,
  };
  useEffect(() => {
    store.upsert(controller.current);
  }, [store, key, status, draft, isDirty, reset, setDraft]);
  useEffect(() => () => store.remove(key), [store, key]);
  return null;
}

/**
 * Keeps one policy contributor per runtime mounted while the Agents route is
 * mounted, independent of the conditionally rendered manager surfaces.
 */
export function RuntimePolicyControllers({
  statuses,
  children,
}: {
  statuses: AgentUpdateStatus[];
  children: ReactNode;
}) {
  const storeRef = useRef<RuntimePolicyStore | null>(null);
  storeRef.current ??= createRuntimePolicyStore();
  return (
    <RuntimePolicyContext.Provider value={storeRef.current}>
      {statuses.map((status) => (
        <RuntimePolicyContributor
          key={`${status.agent_name}:${status.runtime_id}`}
          store={storeRef.current!}
          status={status}
        />
      ))}
      {children}
    </RuntimePolicyContext.Provider>
  );
}

export function useRuntimePolicyController(agentName: string) {
  const store = useContext(RuntimePolicyContext);
  return useSyncExternalStore(
    store?.subscribe ?? (() => () => {}),
    () => store?.get(agentName),
    () => undefined,
  );
}

export function useRuntimePolicyControllers() {
  const store = useContext(RuntimePolicyContext);
  return useSyncExternalStore(
    store?.subscribe ?? (() => () => {}),
    () => store?.list() ?? EMPTY,
    () => EMPTY,
  );
}

export function RuntimePolicyOutcome({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const outcome = status.last_outcome;
  if (!outcome) return null;
  const values = {
    agent: status.display_name,
    previous: outcome.previous_version,
    version: outcome.target_version,
  };
  let key = "agents:runtimeFailedBody";
  if (outcome.status === "running") key = "agents:runtimeRunning";
  if (outcome.status === "interrupted") key = "agents:runtimeInterruptedBody";
  if (outcome.status === "succeeded") key = "agents:runtimeSucceededBody";
  return (
    <p className="text-sm" role="status">
      {t(key, values)}
    </p>
  );
}

export function RuntimeGuidanceAction({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  if (!status.guidance_url) return null;
  return (
    <Button asChild variant="outline" className={settingsActionClassName("shrink-0")}>
      <a href={status.guidance_url} target="_blank" rel="noopener noreferrer">
        {t("agents:runtimeManualGuidance")}
      </a>
    </Button>
  );
}

function ownershipKey(status: AgentUpdateStatus): string {
  if (status.management === "managed") return "agents:runtimeManaged";
  if (status.management === "manual") return "agents:runtimeManual";
  return "agents:runtimeUnsupported";
}

function RuntimePolicyIdentity({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const managed = status.management === "managed";
  return (
    <>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-xs text-muted-foreground">
          {t(managed ? "agents:runtimeSelected" : "agents:runtimeObserved", {
            version: status.effective_version || t("agents:runtimeUnknown"),
          })}
        </span>
        <span className="text-xs text-muted-foreground">
          {t("agents:runtimeLatest", {
            version: status.latest_version || t("agents:runtimeUnknown"),
          })}
        </span>
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="min-w-0 break-all font-mono">
          {t("agents:runtimeVersionSource", { runtime: status.source || status.runtime_id })}
        </span>
        <span>{t(ownershipKey(status))}</span>
      </div>
    </>
  );
}

function RuntimeAutomaticPolicy({
  status,
  draft,
  isDirty,
  canManage,
  onChange,
}: {
  status: AgentUpdateStatus;
  draft: boolean;
  isDirty: boolean;
  canManage: boolean;
  onChange?: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  if (status.management !== "managed") return null;
  const controlId = `runtime-automatic-detail-${status.agent_name}`;
  const helpId = `${controlId}-help`;
  const showHelp = status.available && status.enabled;
  return (
    <>
      <div className="flex items-center justify-between gap-3">
        <label
          className="flex min-h-8 flex-1 cursor-pointer items-center text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          htmlFor={controlId}
          data-testid={`runtime-automatic-target-${status.agent_name}`}
        >
          {t("agents:runtimeAutomatic")}
        </label>
        <Switch
          id={controlId}
          aria-describedby={showHelp ? helpId : undefined}
          checked={draft}
          onCheckedChange={onChange}
          disabled={!canManage || !onChange || (!status.auto_update_supported && !draft)}
        />
      </div>
      {showHelp && (
        <p id={helpId} className="text-xs text-muted-foreground">
          {t("agents:runtimeAutomaticHelp")}
        </p>
      )}
      {isDirty && <span className="sr-only">{t("agents:runtimePolicyDirtyTitle")}</span>}
    </>
  );
}

function RuntimePolicyNotes({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  if (!status.available || !status.enabled) {
    return <p className="text-xs text-muted-foreground">{t("agents:runtimeNotAvailable")}</p>;
  }
  if (status.management === "managed") return null;
  return (
    <p className="text-xs text-muted-foreground">
      {t(
        status.management === "manual"
          ? "agents:runtimeManualHelp"
          : "agents:runtimeUnsupportedHelp",
      )}
    </p>
  );
}

/**
 * The reusable runtime identity and automatic-update controls shown in the
 * manager's detail view. It reads the route-level policy controller so the
 * draft survives overview/detail navigation and overlay dismissal.
 */
export function RuntimePolicyDetail({ status }: { status: AgentUpdateStatus }) {
  const canManage = useIsAdmin();
  const controller = useRuntimePolicyController(status.agent_name);
  const draft = controller?.draft ?? status.auto_update;
  const isDirty = controller?.isDirty ?? false;
  return (
    <div
      className="space-y-2 border-b px-4 py-3"
      data-testid={`runtime-policy-detail-${status.agent_name}`}
      data-settings-dirty={isDirty}
    >
      <RuntimePolicyIdentity status={status} />
      <RuntimeAutomaticPolicy
        status={status}
        draft={draft}
        isDirty={isDirty}
        canManage={canManage}
        onChange={controller?.setDraft}
      />
      <RuntimePolicyNotes status={status} />
      <RuntimePolicyOutcome status={status} />
      <RuntimeGuidanceAction status={status} />
    </div>
  );
}
