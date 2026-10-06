"use client";

import { useState, useCallback, useLayoutEffect, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

export type FileReviewState = {
  reviewed: boolean;
  diffHash: string;
};

type SessionFileReview = {
  id: string;
  session_id: string;
  file_path: string;
  reviewed: boolean;
  diff_hash: string;
  reviewed_at: string | null;
  created_at: string;
  updated_at: string;
};

type UseSessionFileReviewsReturn = {
  reviews: Map<string, FileReviewState>;
  markReviewed: (filePath: string, diffHash: string) => void;
  markUnreviewed: (filePath: string) => void;
  resetReviews: () => void;
  loading: boolean;
};

// Shared module-level cache so all hook instances share the same review state
const reviewsCache: Record<string, Map<string, FileReviewState>> = {};
const fetchedSessions = new Set<string>();
let cacheVersion = 0;

function notifyChange() {
  cacheVersion++;
  window.dispatchEvent(new CustomEvent("file-reviews-change"));
}

/** Update shared cache with a new map and notify other hook instances. */
function updateCache(sessionId: string, map: Map<string, FileReviewState>) {
  reviewsCache[sessionId] = map;
  notifyChange();
}

/** Create an optimistic update: clone the cache, apply mutation, update cache + local state. */
function optimisticUpdate(
  sessionId: string,
  mutate: (next: Map<string, FileReviewState>) => void,
  setReviews: (m: Map<string, FileReviewState>) => void,
) {
  const next = new Map(reviewsCache[sessionId] ?? new Map());
  mutate(next);
  reviewsCache[sessionId] = next;
  setReviews(next);
  notifyChange();
}

function fetchSessionReviews(
  sessionId: string,
  setReviews: (m: Map<string, FileReviewState>) => void,
  setLoading: (v: boolean) => void,
) {
  const client = getWebSocketClient();
  if (!client) return;

  fetchedSessions.add(sessionId);
  queueMicrotask(() => setLoading(true));

  client
    .request<{ reviews: SessionFileReview[] }>("session.file_review.get", { session_id: sessionId })
    .then((response) => {
      const map = new Map<string, FileReviewState>();
      if (response?.reviews) {
        for (const review of response.reviews) {
          map.set(review.file_path, { reviewed: review.reviewed, diffHash: review.diff_hash });
        }
      }
      updateCache(sessionId, map);
      setReviews(map);
    })
    .catch(() => {
      /* Ignore errors - reviews are not critical */
    })
    .finally(() => {
      setLoading(false);
    });
}

type ReaderOwner = { sessionId: string | null; active: boolean };
type ReaderSnapshot = {
  owner: ReaderOwner | null;
  reviews: Map<string, FileReviewState>;
  loading: boolean;
};
const emptyReviews = new Map<string, FileReviewState>();

function readerPublisher(
  owner: ReaderOwner | null,
  setSnapshot: React.Dispatch<React.SetStateAction<ReaderSnapshot>>,
) {
  const publish = (patch: Partial<Pick<ReaderSnapshot, "reviews" | "loading">>) => {
    if (!owner?.active) return;
    setSnapshot((previous) => {
      if (!owner.active) return previous;
      const current =
        previous.owner === owner
          ? previous
          : {
              owner,
              reviews: owner.sessionId
                ? (reviewsCache[owner.sessionId] ?? emptyReviews)
                : emptyReviews,
              loading: false,
            };
      return { ...current, ...patch };
    });
  };
  return {
    setReviews: (reviews: Map<string, FileReviewState>) => publish({ reviews }),
    setLoading: (loading: boolean) => publish({ loading }),
  };
}

function useReviewReader(sessionId: string | null) {
  const [snapshot, setSnapshot] = useState<ReaderSnapshot>({
    owner: null,
    reviews: emptyReviews,
    loading: false,
  });
  const ownerRef = useRef<ReaderOwner | null>(null);
  const versionRef = useRef(cacheVersion);

  useLayoutEffect(() => {
    const owner: ReaderOwner = { sessionId, active: true };
    ownerRef.current = owner;
    const publisher = readerPublisher(owner, setSnapshot);
    versionRef.current = cacheVersion;
    queueMicrotask(() => {
      publisher.setReviews(sessionId ? (reviewsCache[sessionId] ?? emptyReviews) : emptyReviews);
      publisher.setLoading(false);
    });
    const handler = () => {
      if (!sessionId || !owner.active) return;
      const cached = reviewsCache[sessionId];
      if (cached && cacheVersion !== versionRef.current) {
        versionRef.current = cacheVersion;
        publisher.setReviews(cached);
      }
    };
    window.addEventListener("file-reviews-change", handler);
    if (sessionId && !fetchedSessions.has(sessionId)) {
      fetchSessionReviews(sessionId, publisher.setReviews, publisher.setLoading);
    }
    return () => {
      owner.active = false;
      window.removeEventListener("file-reviews-change", handler);
    };
  }, [sessionId]);

  const capturePublisher = useCallback(() => {
    const owner = ownerRef.current;
    return readerPublisher(owner?.sessionId === sessionId ? owner : null, setSnapshot);
  }, [sessionId]);
  const current = snapshot.owner?.active && snapshot.owner.sessionId === sessionId;
  const fallbackReviews = sessionId ? (reviewsCache[sessionId] ?? emptyReviews) : emptyReviews;
  return {
    reviews: current ? snapshot.reviews : fallbackReviews,
    loading: current ? snapshot.loading : false,
    capturePublisher,
  };
}

export function useSessionFileReviews(sessionId: string | null): UseSessionFileReviewsReturn {
  const { reviews, loading, capturePublisher } = useReviewReader(sessionId);

  const markReviewed = useCallback(
    (filePath: string, diffHash: string) => {
      if (!sessionId) return;
      const { setReviews } = capturePublisher();
      optimisticUpdate(
        sessionId,
        (next) => {
          next.set(filePath, { reviewed: true, diffHash });
        },
        setReviews,
      );
      const client = getWebSocketClient();
      if (!client) return;
      client
        .request("session.file_review.update", {
          session_id: sessionId,
          file_path: filePath,
          reviewed: true,
          diff_hash: diffHash,
        })
        .catch(() => {
          optimisticUpdate(
            sessionId,
            (reverted) => {
              reverted.delete(filePath);
            },
            setReviews,
          );
        });
    },
    [sessionId, capturePublisher],
  );

  const markUnreviewed = useCallback(
    (filePath: string) => {
      if (!sessionId) return;
      const { setReviews } = capturePublisher();
      optimisticUpdate(
        sessionId,
        (next) => {
          next.set(filePath, { reviewed: false, diffHash: "" });
        },
        setReviews,
      );
      const client = getWebSocketClient();
      if (!client) return;
      client
        .request("session.file_review.update", {
          session_id: sessionId,
          file_path: filePath,
          reviewed: false,
          diff_hash: "",
        })
        .catch(() => {
          /* Ignore failures for unmark */
        });
    },
    [sessionId, capturePublisher],
  );

  const resetReviews = useCallback(() => {
    if (!sessionId) return;
    const { setReviews } = capturePublisher();
    reviewsCache[sessionId] = new Map();
    setReviews(new Map());
    notifyChange();
    const client = getWebSocketClient();
    if (!client) return;
    client.request("session.file_review.reset", { session_id: sessionId }).catch(() => {
      /* Ignore */
    });
  }, [sessionId, capturePublisher]);

  return { reviews, markReviewed, markUnreviewed, resetReviews, loading };
}
