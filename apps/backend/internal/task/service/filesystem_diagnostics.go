package service

import "github.com/kandev/kandev/internal/common/fsdiagnostics"

func (s *Service) filesystemContext(operation, target, trigger string) fsdiagnostics.Context {
	return fsdiagnostics.Context{
		Operation: operation,
		Target:    target,
		Trigger:   trigger,
		Runtime:   fsdiagnostics.RuntimeMode(s.discoveryConfig.DesktopRuntime),
	}
}

func (s *Service) logFilesystemFailure(operation, target, trigger string, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	operationContext := s.filesystemContext(operation, target, trigger)
	if fsdiagnostics.IsAccessDenied(err) {
		s.filesystemWarnings.Warn(s.logger.Zap(), "filesystem.access_denied", operationContext, err)
		return
	}
	s.logger.Warn("filesystem operation failed", operationContext.Fields(err)...)
}

func (s *Service) logFilesystemInfo(message, operation, target, trigger string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(message, s.filesystemContext(operation, target, trigger).Fields(nil)...)
}
