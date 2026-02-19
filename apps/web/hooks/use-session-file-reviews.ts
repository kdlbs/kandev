"use client";

import { useState, useCallback, useEffect, useRef } from "react";
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

export function useSessionFileReviews(sessionId: string | null): UseSessionFileReviewsReturn {
  const [reviews, setReviews] = useState<Map<string, FileReviewState>>(() =>
    sessionId ? (reviewsCache[sessionId] ?? new Map()) : new Map(),
  );
  const [loading, setLoading] = useState(false);
  const versionRef = useRef(cacheVersion);

  useEffect(() => {
    const handler = () => {
      if (!sessionId) return;
      const cached = reviewsCache[sessionId];
      if (cached && cacheVersion !== versionRef.current) {
        versionRef.current = cacheVersion;
        setReviews(cached);
      }
    };
    window.addEventListener("file-reviews-change", handler);
    return () => window.removeEventListener("file-reviews-change", handler);
  }, [sessionId]);

  useEffect(() => {
    if (!sessionId || fetchedSessions.has(sessionId)) {
      if (sessionId && reviewsCache[sessionId]) {
        const cached = reviewsCache[sessionId];
        queueMicrotask(() => setReviews(cached));
      }
      return;
    }
    fetchSessionReviews(sessionId, setReviews, setLoading);
  }, [sessionId]);

  const markReviewed = useCallback(
    (filePath: string, diffHash: string) => {
      if (!sessionId) return;
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
    [sessionId],
  );

  const markUnreviewed = useCallback(
    (filePath: string) => {
      if (!sessionId) return;
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
    [sessionId],
  );

  const resetReviews = useCallback(() => {
    if (!sessionId) return;
    reviewsCache[sessionId] = new Map();
    setReviews(new Map());
    notifyChange();
    const client = getWebSocketClient();
    if (!client) return;
    client.request("session.file_review.reset", { session_id: sessionId }).catch(() => {
      /* Ignore */
    });
  }, [sessionId]);

  return { reviews, markReviewed, markUnreviewed, resetReviews, loading };
}
