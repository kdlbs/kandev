package sandboxruntime

import (
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
)

func projectReportOperation(operation *pluginsdk.SandboxOperationV2) (string, policy.ReportOperation) {
	var result policy.ReportOperation
	switch value := operation.GetValue().(type) {
	case *pluginsdk.SandboxOperationV2_Filesystem:
		result.Filesystem = &policy.ReportFilesystem{Path: value.Filesystem.Path, Access: value.Filesystem.Access}
		return reportClassFilesystem, result
	case *pluginsdk.SandboxOperationV2_Tcp:
		result.TCP = &policy.ReportTCP{Host: value.Tcp.Host, Port: value.Tcp.Port, Transport: value.Tcp.Transport}
		return reportClassNetwork, result
	case *pluginsdk.SandboxOperationV2_Command:
		result.Command = &policy.ReportCommand{ID: value.Command.CommandId, Caller: value.Command.Caller}
		return reportClassCommand, result
	case *pluginsdk.SandboxOperationV2_HttpEndpoint:
		result.Endpoint = &policy.ReportEndpoint{RouteID: value.HttpEndpoint.RouteId, Origin: value.HttpEndpoint.Origin, Method: value.HttpEndpoint.Method, Path: value.HttpEndpoint.Path}
		return "http_endpoint", result
	default:
		return "", result
	}
}
