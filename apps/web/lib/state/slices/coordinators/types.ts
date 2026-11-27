import type { Coordinator } from "@/lib/api/domains/coordinator-api";

export type CoordinatorsState = {
  items: Coordinator[];
  loaded: boolean;
  loading: boolean;
};

export type CoordinatorsSliceState = {
  coordinators: CoordinatorsState;
};

export type CoordinatorsSliceActions = {
  setCoordinators: (items: Coordinator[]) => void;
  setCoordinatorsLoading: (loading: boolean) => void;
  addCoordinator: (coordinator: Coordinator) => void;
  updateCoordinator: (coordinator: Coordinator) => void;
  removeCoordinator: (id: string) => void;
};

export type CoordinatorsSlice = CoordinatorsSliceState & CoordinatorsSliceActions;
