import { useCallback, useRef, useState } from "react";

export type StorageSection = "policy" | "overview" | "disk" | "runs" | "quarantine";
export type StorageSectionLoading = Record<StorageSection, boolean>;
export type StorageSectionErrors = Record<StorageSection, string | null>;

export function useStorageSections() {
  const [loading, setLoading] = useState<StorageSectionLoading>({
    policy: true,
    overview: true,
    disk: true,
    runs: true,
    quarantine: true,
  });
  const [sectionErrors, setSectionErrors] = useState<StorageSectionErrors>({
    policy: null,
    overview: null,
    disk: null,
    runs: null,
    quarantine: null,
  });
  const sectionGenerations = useRef<Record<StorageSection, number>>({
    policy: 0,
    overview: 0,
    disk: 0,
    runs: 0,
    quarantine: 0,
  });
  const loadSection = useCallback(
    async <T>(section: StorageSection, request: () => Promise<T>, commit: (value: T) => void) => {
      const generation = ++sectionGenerations.current[section];
      setLoading((current) => ({ ...current, [section]: true }));
      setSectionErrors((current) => ({ ...current, [section]: null }));
      try {
        const value = await request();
        if (generation === sectionGenerations.current[section]) commit(value);
      } catch (requestError) {
        if (generation === sectionGenerations.current[section]) {
          const message =
            requestError instanceof Error ? requestError.message : String(requestError);
          setSectionErrors((current) => ({ ...current, [section]: message }));
          throw requestError;
        }
      } finally {
        if (generation === sectionGenerations.current[section]) {
          setLoading((current) => ({ ...current, [section]: false }));
        }
      }
    },
    [],
  );
  return { loading, setLoading, sectionErrors, setSectionErrors, sectionGenerations, loadSection };
}
