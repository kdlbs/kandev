"use client";

import { useCallback, useEffect, useState } from "react";
import type {
  ModelCatalog,
  ModelCatalogComposition,
  ModelCatalogDraft,
} from "@kandev/types/model-catalog";

import { catalogComposition } from "@/lib/model-catalog";
import {
  composeCatalogSelection,
  initialCompositionDraft,
  type CatalogGroupEntry,
} from "@/lib/model-catalog-selection";

/** One completed composed selection. Dimensions stay presentation-only. */
export type CompletedModelConfiguration = {
  model: string;
  configOptions: Record<string, string>;
};

/**
 * A real provider configuration option for a candidate model. Structurally
 * compatible with the selector's `DynamicConfigOption`.
 */
export type ConfigurationOption = {
  type: string;
  id: string;
  name: string;
  description?: string;
  currentValue: string;
  category?: string;
  options?: { value: string; name: string; description?: string }[];
};

export type CandidateOptionsState = {
  status: "idle" | "loading" | "ready" | "failed";
  options: ConfigurationOption[];
  /** Model whose snapshot produced this state. Apply requires an exact match. */
  model?: string;
};

export type ModelCatalogNavigation = {
  activeComposition: ModelCatalogComposition | undefined;
  activeDimensionId: string | null;
  draft: ModelCatalogDraft;
  openComposition: (group: CatalogGroupEntry) => void;
  closeComposition: () => void;
  openDimension: (dimensionId: string) => void;
  closeDimension: () => void;
  setDimension: (dimensionId: string, value: string) => void;
  apply: () => void;
  applyEnabled: boolean;
  candidateOptions: CandidateOptionsState;
  candidateOptionValues: Record<string, string>;
  setCandidateOption: (optionId: string, value: string) => void;
  retryCandidateOptions: () => void;
};

type Options = {
  catalog: ModelCatalog | undefined;
  currentModelValue: string;
  onApply: (selection: CompletedModelConfiguration) => void;
  /** Runs after a completed apply so the caller can close the picker. */
  onApplied?: () => void;
  /** Resolves a candidate model's authoritative provider options without mutation. */
  resolveConfigurationOptions?: (model: string) => Promise<ConfigurationOption[] | undefined>;
};

const IDLE_CANDIDATE_OPTIONS: CandidateOptionsState = { status: "idle", options: [] };

function selectableOptions(options: ConfigurationOption[]): ConfigurationOption[] {
  return options.filter(
    (option) =>
      option.type === "select" &&
      option.id !== "model" &&
      option.category !== "model" &&
      option.id !== "mode" &&
      option.category !== "mode" &&
      Array.isArray(option.options) &&
      option.options.length > 0,
  );
}

function defaultOptionValues(options: ConfigurationOption[]): Record<string, string> {
  return Object.fromEntries(options.map((option) => [option.id, option.currentValue]));
}

function useDraftDimensionSetter(
  setActiveGroupId: React.Dispatch<React.SetStateAction<string | null>>,
  setDrafts: React.Dispatch<React.SetStateAction<Record<string, ModelCatalogDraft>>>,
) {
  return useCallback(
    (dimensionId: string, value: string) => {
      setActiveGroupId((groupId) => {
        if (!groupId) return groupId;
        setDrafts((previous) => ({
          ...previous,
          [groupId]: { ...(previous[groupId] ?? {}), [dimensionId]: value },
        }));
        return groupId;
      });
    },
    [setActiveGroupId, setDrafts],
  );
}

function canApplyComposition(
  resolution: ReturnType<typeof composeCatalogSelection> | undefined,
  resolver: ((model: string) => Promise<ConfigurationOption[] | undefined>) | undefined,
  options: CandidateOptionsState,
  model: string | undefined,
): boolean {
  if (resolution?.status !== "complete") return false;
  return !resolver || (options.status === "ready" && options.model === model);
}

/**
 * Resolves one candidate model's authoritative provider options. The request is
 * fenced by model identity and a retry generation; nothing hidden or stale is
 * published to the picker.
 */
function useResolvedCandidateOptions(
  resolvedModel: string | undefined,
  resolver: ((model: string) => Promise<ConfigurationOption[] | undefined>) | undefined,
  retry: number,
): {
  state: CandidateOptionsState;
  values: Record<string, string>;
  setValue: (optionId: string, value: string) => void;
} {
  const [state, setState] = useState<CandidateOptionsState>(IDLE_CANDIDATE_OPTIONS);
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => {
    setValues({});
    if (!resolvedModel || !resolver) {
      setState(IDLE_CANDIDATE_OPTIONS);
      return;
    }
    let cancelled = false;
    setState({ status: "loading", options: [], model: resolvedModel });
    void resolver(resolvedModel)
      .then((resolved) => {
        if (cancelled) return;
        if (!resolved) {
          setState({ status: "failed", options: [], model: resolvedModel });
          return;
        }
        const options = selectableOptions(resolved);
        setState({ status: "ready", options, model: resolvedModel });
        setValues(defaultOptionValues(options));
      })
      .catch(() => {
        if (cancelled) return;
        setState({ status: "failed", options: [], model: resolvedModel });
      });
    return () => {
      cancelled = true;
    };
  }, [resolvedModel, resolver, retry]);
  const setValue = useCallback((optionId: string, value: string) => {
    setValues((previous) => ({ ...previous, [optionId]: value }));
  }, []);
  return { state, values, setValue };
}

/**
 * Owns the composed-selection draft, its resolved provider options, and the
 * picker's navigation position. The unfinished draft is kept per group while
 * the user navigates Back to the model list, and a superseded catalog or a new
 * owning model invalidates it instead of silently reusing stale values.
 */
export function useModelCatalogNavigation({
  catalog,
  currentModelValue,
  onApply,
  onApplied,
  resolveConfigurationOptions,
}: Options): ModelCatalogNavigation | undefined {
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [activeDimensionId, setActiveDimensionId] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, ModelCatalogDraft>>({});
  const [optionRetry, setOptionRetry] = useState(0);
  const setDimension = useDraftDimensionSetter(setActiveGroupId, setDrafts);

  const activeComposition = activeGroupId ? catalogComposition(catalog, activeGroupId) : undefined;
  const draft = activeGroupId ? (drafts[activeGroupId] ?? {}) : {};
  const resolution =
    activeComposition && activeGroupId
      ? composeCatalogSelection(activeComposition, draft, catalog)
      : undefined;
  const resolvedModel = resolution?.status === "complete" ? resolution.model : undefined;

  // A new owning model or a different catalog invalidates every unfinished
  // draft: a stale pairing must never survive into a different catalog.
  useEffect(() => {
    setDrafts({});
    setActiveDimensionId(null);
  }, [catalog, currentModelValue]);

  const {
    state: candidateOptions,
    values: candidateOptionValues,
    setValue: setCandidateOption,
  } = useResolvedCandidateOptions(resolvedModel, resolveConfigurationOptions, optionRetry);

  const openComposition = useCallback(
    (group: CatalogGroupEntry) => {
      const composition = catalogComposition(catalog, group.id);
      if (!composition) return;
      setActiveGroupId(group.id);
      setActiveDimensionId(null);
      setDrafts((previous) =>
        previous[group.id]
          ? previous
          : { ...previous, [group.id]: initialCompositionDraft(composition, currentModelValue) },
      );
    },
    [catalog, currentModelValue],
  );

  const apply = useCallback(() => {
    if (!activeGroupId || resolution?.status !== "complete") return;
    const optionsReady =
      !resolveConfigurationOptions ||
      (candidateOptions.status === "ready" && candidateOptions.model === resolvedModel);
    if (!optionsReady) return;
    onApply({
      model: resolution.model,
      configOptions: { ...candidateOptionValues },
    });
    setDrafts((previous) => {
      const next = { ...previous };
      delete next[activeGroupId];
      return next;
    });
    setActiveGroupId(null);
    setActiveDimensionId(null);
    onApplied?.();
  }, [
    activeGroupId,
    resolution,
    resolveConfigurationOptions,
    candidateOptions.status,
    candidateOptionValues,
    onApply,
    onApplied,
  ]);

  const applyEnabled = canApplyComposition(
    resolution,
    resolveConfigurationOptions,
    candidateOptions,
    resolvedModel,
  );

  if (!catalog) return undefined;
  return {
    activeComposition,
    activeDimensionId,
    draft,
    openComposition,
    closeComposition: () => {
      setActiveDimensionId(null);
      setActiveGroupId(null);
    },
    openDimension: setActiveDimensionId,
    closeDimension: () => setActiveDimensionId(null),
    setDimension,
    apply,
    applyEnabled,
    candidateOptions,
    candidateOptionValues,
    setCandidateOption,
    retryCandidateOptions: () => setOptionRetry((previous) => previous + 1),
  };
}
