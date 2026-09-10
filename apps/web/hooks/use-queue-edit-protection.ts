import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import {
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  renewQueuedMessageEdit,
  type QueueEditLease,
} from "@/lib/api/domains/queue-api";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

type QueueEditProtectionArgs = {
  sessionId: string | null;
  entries: QueuedMessage[];
};

type ActiveEdit = {
  sessionId: string;
  entryId: string;
  editToken: string;
  lease: QueueEditLease;
  renewalSequence: number;
};

const EDIT_RENEW_INTERVAL_MS = 20_000;

type RenewalOwnershipArgs = {
  activeEdit: ActiveEdit;
  currentEdit: ActiveEdit | null;
  leaseSnapshot: QueueEditLease;
  renewedLease: QueueEditLease;
  renewalSequence: number;
};

function ownsRenewal({
  activeEdit,
  currentEdit,
  leaseSnapshot,
  renewedLease,
  renewalSequence,
}: RenewalOwnershipArgs): boolean {
  const currentLease = currentEdit?.lease;
  if (
    !currentEdit ||
    currentEdit.sessionId !== activeEdit.sessionId ||
    currentEdit.entryId !== activeEdit.entryId ||
    currentLease?.lease_id !== leaseSnapshot.lease_id
  ) {
    return false;
  }
  if (
    currentEdit.renewalSequence !== renewalSequence &&
    (currentLease.lease_generation === undefined || renewedLease.lease_generation === undefined)
  ) {
    return false;
  }
  return !(
    currentLease.lease_generation !== undefined &&
    (renewedLease.lease_generation === undefined ||
      renewedLease.lease_generation < currentLease.lease_generation)
  );
}

/** Acquires a target-bound server lease before activating a queue editor. */
// eslint-disable-next-line max-lines-per-function -- coordinates the full lease lifecycle.
export function useQueueEditProtection({ sessionId, entries }: QueueEditProtectionArgs) {
  const { t } = useTranslation();
  const [editingEntryId, setEditingEntryId] = useState<string | null>(null);
  const [editLease, setEditLease] = useState<QueueEditLease | null>(null);
  const activeEditRef = useRef<ActiveEdit | null>(null);
  const nextEditTokenRef = useRef(0);
  const acquiringEditRef = useRef<{ sessionId: string; entryId: string } | null>(null);
  const mountedRef = useRef(false);
  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;
  const entriesRef = useRef(entries);
  entriesRef.current = entries;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  const beginEdit = useCallback(
    async (entryId: string): Promise<string | false> => {
      if (!sessionId || editingEntryId || activeEditRef.current || acquiringEditRef.current) {
        return false;
      }
      const acquisition = { sessionId, entryId };
      acquiringEditRef.current = acquisition;
      try {
        const lease = await beginQueuedMessageEdit(sessionId, entryId);
        if (
          !mountedRef.current ||
          sessionIdRef.current !== sessionId ||
          !entriesRef.current.some((entry) => entry.id === entryId)
        ) {
          await endQueuedMessageEdit(lease).catch(() => undefined);
          return false;
        }
        const editToken = `queue-edit-${++nextEditTokenRef.current}`;
        activeEditRef.current = { sessionId, entryId, editToken, lease, renewalSequence: 0 };
        setEditLease(lease);
        setEditingEntryId(entryId);
        return editToken;
      } catch (err) {
        console.error("Failed to acquire queued message edit lease:", err);
        toast.error(t("chat:queueEditSaveFailed"));
        return false;
      } finally {
        if (acquiringEditRef.current === acquisition) acquiringEditRef.current = null;
      }
    },
    [editingEntryId, entries, sessionId, t],
  );

  const completeEdit = useCallback(
    async (
      entryId: string,
      expectedSessionId = sessionIdRef.current,
      expectedEditToken?: string,
      dispatchIfAutoRun = false,
    ): Promise<void> => {
      const activeEdit = activeEditRef.current;
      if (
        !activeEdit ||
        activeEdit.entryId !== entryId ||
        activeEdit.sessionId !== expectedSessionId ||
        (expectedEditToken !== undefined && activeEdit.editToken !== expectedEditToken)
      ) {
        return;
      }
      activeEditRef.current = null;
      setEditLease(null);
      setEditingEntryId(null);
      const release = dispatchIfAutoRun
        ? endQueuedMessageEdit(activeEdit.lease, true)
        : endQueuedMessageEdit(activeEdit.lease);
      await release.catch((err) => {
        console.error("Failed to release queued message edit lease:", err);
      });
    },
    [],
  );

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit || activeEdit.sessionId !== sessionId) return;
    const renew = async () => {
      const leaseSnapshot = activeEdit.lease;
      const renewalSequence = activeEdit.renewalSequence + 1;
      activeEdit.renewalSequence = renewalSequence;
      try {
        const lease = await renewQueuedMessageEdit(leaseSnapshot);
        const currentEdit = activeEditRef.current;
        if (
          !ownsRenewal({
            activeEdit,
            currentEdit,
            leaseSnapshot,
            renewedLease: lease,
            renewalSequence,
          })
        ) {
          return;
        }
        if (!currentEdit) return;
        currentEdit.lease = lease;
        setEditLease(lease);
      } catch (err) {
        // A renewal can reject after this edit has been completed or a newer
        // renewal has replaced its lease snapshot. Only the renewal that
        // still owns the active snapshot may clear the edit.
        if (
          activeEditRef.current !== activeEdit ||
          activeEdit.renewalSequence !== renewalSequence
        ) {
          return;
        }
        console.error("Queued message edit lease renewal failed:", err);
        await completeEdit(activeEdit.entryId);
        toast.error(t("chat:queueEditSaveFailed"));
      }
    };
    const timer = window.setInterval(() => void renew(), EDIT_RENEW_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [completeEdit, editingEntryId, t]);

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (
      !activeEdit ||
      activeEdit.sessionId !== sessionId ||
      entries.some((entry) => entry.id === activeEdit.entryId)
    ) {
      return;
    }
    void completeEdit(activeEdit.entryId, activeEdit.sessionId);
  }, [completeEdit, entries, sessionId]);

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit || activeEdit.sessionId === sessionId) return;
    activeEditRef.current = null;
    setEditingEntryId(null);
    setEditLease(null);
    void endQueuedMessageEdit(activeEdit.lease).catch((err) => {
      console.error("Failed to release queued message edit lease:", err);
    });
  }, [sessionId]);

  useEffect(
    () => () => {
      const activeEdit = activeEditRef.current;
      activeEditRef.current = null;
      if (activeEdit) {
        void endQueuedMessageEdit(activeEdit.lease).catch((err) => {
          console.error("Failed to release queued message edit lease:", err);
        });
      }
    },
    [],
  );

  return { editingEntryId, editLease, beginEdit, completeEdit };
}
type QueuedGhostEditStartArgs = {
  canEdit: boolean;
  editing: boolean;
  saving: boolean;
  onEditStart?: () => void | Promise<boolean | string | void>;
  onStart: (editToken?: string) => void;
};

export function useQueuedGhostStartEdit({
  canEdit,
  editing,
  saving,
  onEditStart,
  onStart,
}: QueuedGhostEditStartArgs) {
  const editStartingRef = useRef(false);
  return useCallback(async () => {
    if (!canEdit || editing || saving || editStartingRef.current) return;
    editStartingRef.current = true;
    try {
      const editToken = onEditStart ? await onEditStart() : undefined;
      if (editToken === false) return;
      onStart(typeof editToken === "string" ? editToken : undefined);
    } finally {
      editStartingRef.current = false;
    }
  }, [canEdit, editing, onEditStart, onStart, saving]);
}
