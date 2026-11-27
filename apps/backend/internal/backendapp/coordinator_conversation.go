package backendapp

// wireCoordinatorConversation supplies the coordinator service's conversation
// route with its task-service and orchestrator dependencies, and its
// archive/delete lifecycle hooks (copilot.md#wiring). It runs from
// registerSecondaryRoutes, inside the existing features.coordinator guard, on
// the line before registerCoordinatorRoutes(p): the route registration in
// registerCoordinatorConversation reads these dependencies off svc, so they
// must already be set by the time that call happens.
func wireCoordinatorConversation(p routeParams) {
	if p.services == nil || p.services.Coordinator == nil || p.orchestratorSvc == nil || p.taskSvc == nil {
		return
	}
	svc := p.services.Coordinator
	svc.SetConversationDeps(p.taskSvc, p.orchestratorSvc)
	svc.SetConversationHooks(svc.ArchiveClearedConversationTask, svc.DeleteConversationTasksForCoordinator)
}
