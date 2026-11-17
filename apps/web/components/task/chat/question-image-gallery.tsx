"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import type { ClarificationQuestion } from "@/lib/types/http";
import { buildSegmentImages, imageShortName } from "@/lib/question-review/segments";

type Registration = {
  id: string;
  url: string;
  alt: string;
  trigger: RefObject<HTMLElement | null>;
};

export type GalleryItem = Registration & { name: string; hasLabel: boolean };

type Gallery = {
  items: readonly GalleryItem[];
  register: (entry: Registration) => () => void;
};

const GalleryContext = createContext<Gallery | null>(null);

/**
 * Collects the loaded thumbnails of one question (its own images, then choice
 * images in option order) so the inspector can step between them.
 */
export function QuestionImageGallery({
  question,
  children,
}: {
  question: ClarificationQuestion;
  children: ReactNode;
}) {
  const order = useMemo(() => buildSegmentImages(question), [question]);
  const [entries, setEntries] = useState<Record<string, Registration>>({});
  const register = useCallback((entry: Registration) => {
    setEntries((current) => ({ ...current, [entry.id]: entry }));
    return () =>
      setEntries((current) => {
        if (current[entry.id] !== entry) return current;
        const next = { ...current };
        delete next[entry.id];
        return next;
      });
  }, []);
  const items = useMemo(
    () =>
      order.flatMap((image, index) => {
        const entry = entries[image.id];
        if (!entry) return [];
        return [{ ...entry, name: imageShortName(image, index), hasLabel: Boolean(image.label) }];
      }),
    [entries, order],
  );
  const value = useMemo(() => ({ items, register }), [items, register]);
  return <GalleryContext.Provider value={value}>{children}</GalleryContext.Provider>;
}

export function useQuestionImageGallery(): Gallery | null {
  return useContext(GalleryContext);
}

/** Adds a loaded thumbnail to the surrounding gallery, if there is one. */
export function useGalleryRegistration(
  imageId: string | undefined,
  url: string,
  alt: string,
  trigger: RefObject<HTMLElement | null>,
) {
  const gallery = useQuestionImageGallery();
  const register = gallery?.register;
  useEffect(() => {
    if (!register || !imageId) return;
    return register({ id: imageId, url, alt, trigger });
  }, [alt, imageId, register, trigger, url]);
}
