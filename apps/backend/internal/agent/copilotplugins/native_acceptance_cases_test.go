//go:build copilot_native

package copilotplugins_test

import (
	"context"
	"strings"
)

func runNativeCoexistenceCase(
	ctx context.Context,
	version string,
	baseArgs []string,
	env []string,
	paths nativeRunPaths,
	fixtures nativePluginFixture,
) (result nativeACPCaseResult) {
	recorder, err := newNativeRecorder(paths, "coexistence")
	if err != nil {
		return nativeACPCaseResult{reason: "private ACP capture could not be created"}
	}
	defer func() { _ = recorder.Close() }()
	sentinels := newNativeTestSentinels(recorder)
	defer sentinels.close()
	if err := configurePluginMCPFixtures(fixtures, sentinels); err != nil {
		return nativeACPCaseResult{reason: "private plugin MCP fixture could not be written"}
	}

	known := []string{nativeTargetServer, nativeSiblingServer, nativeConfigServer, nativeACPServer}
	driver, err := startNativeACPDriver(
		nativeACPArgs(version, paths, baseArgs, fixtures, sentinels, "", true, known),
		paths.workspace,
		env,
		recorder,
		sentinels,
		known,
		known,
	)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeStartReason(err)}
	}
	defer func() { _ = driver.close() }()
	defer func() {
		captureNativeCaseEvidence(&result, driver, sentinels, known, "", false)
	}()

	actualVersion, httpMCP, sseMCP, err := driver.initialize(ctx)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	if actualVersion != version {
		return nativeACPCaseResult{reason: "ACP executable version did not match the exact package"}
	}
	if !httpMCP || !sseMCP {
		return nativeACPCaseResult{reason: "ACP initialize did not advertise both HTTP and SSE MCP support"}
	}
	sessionID, err := driver.newSession(ctx, paths.workspace, map[string]any{
		"name":    nativeACPServer,
		"type":    "sse",
		"url":     sentinels.servers[nativeACPServer].URL(),
		"headers": []any{},
	})
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	prompt := "Call each exact tool exactly once: " +
		strings.Join(nativeToolNames(known...), ", ") +
		". These are harmless read-only sentinels. Do not call any other tool, read or write files, or run commands. Return only the four tool results."
	if err := driver.prompt(ctx, sessionID, prompt); err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}

	for _, server := range known {
		summary := sentinels.summary(server)
		if !summary.InitializeObserved || !summary.ToolsListObserved || !summary.ToolCallObserved {
			return nativeACPCaseResult{reason: "a required plugin, native-config, or ACP-injected MCP sentinel did not observe initialize/list/call traffic"}
		}
		if summary.ToolCallRequestCount != 1 || summary.ToolCallResultSentCount != 1 {
			return nativeACPCaseResult{reason: "a sentinel did not receive exactly one tool call and send exactly one successful MCP result"}
		}
		if driver.callCount(server) > 0 && !driver.hasSuccessfulSentinelCall(server) {
			return nativeACPCaseResult{reason: "the ACP tool update did not report a successful sentinel result"}
		}
	}
	if len(driver.unexpected) != 0 {
		return nativeACPCaseResult{reason: "the native ACP client requested an unexpected tool or filesystem capability"}
	}
	if driver.captureFailed() {
		return nativeACPCaseResult{reason: "private ACP protocol capture could not be completed"}
	}
	return nativeACPCaseResult{passed: true}
}

func runNativeDisableCase(
	ctx context.Context,
	version string,
	baseArgs []string,
	env []string,
	paths nativeRunPaths,
	fixtures nativePluginFixture,
) (result nativeACPCaseResult) {
	recorder, err := newNativeRecorder(paths, "native-disable")
	if err != nil {
		return nativeACPCaseResult{reason: "private ACP capture could not be created"}
	}
	defer func() { _ = recorder.Close() }()
	sentinels := newNativeTestSentinels(recorder)
	defer sentinels.close()
	if err := configurePluginMCPFixtures(fixtures, sentinels); err != nil {
		return nativeACPCaseResult{reason: "private plugin MCP fixture could not be written"}
	}

	driver, err := startNativeACPDriver(
		nativeACPArgs(version, paths, baseArgs, fixtures, sentinels, nativeTargetServer, false, []string{
			nativeTargetServer,
			nativeSiblingServer,
		}),
		paths.workspace,
		env,
		recorder,
		sentinels,
		[]string{nativeTargetServer, nativeSiblingServer},
		[]string{nativeSiblingServer},
	)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeStartReason(err)}
	}
	defer func() { _ = driver.close() }()
	defer func() {
		captureNativeCaseEvidence(
			&result,
			driver,
			sentinels,
			[]string{nativeTargetServer, nativeSiblingServer},
			nativeTargetServer,
			false,
		)
	}()

	actualVersion, httpMCP, _, err := driver.initialize(ctx)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	if actualVersion != version || !httpMCP {
		return nativeACPCaseResult{reason: "native-disable control did not start the exact ACP executable with HTTP MCP support"}
	}
	sessionID, err := driver.newSession(ctx, paths.workspace, nil)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	prompt := "Call each exact tool exactly once, in this order: " +
		strings.Join(nativeToolNames(nativeTargetServer, nativeSiblingServer), ", then ") +
		". These are harmless read-only sentinels. Do not call any other tool, read or write files, or run commands."
	if err := driver.prompt(ctx, sessionID, prompt); err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	if driver.wasHarnessRejected(nativeTargetServer) {
		return nativeACPCaseResult{reason: "the ACP harness rejected the target permission, which is not native-disable evidence"}
	}
	targetSummary := sentinels.summary(nativeTargetServer)
	if targetSummary.ToolCallRequestCount > 0 || targetSummary.ToolCallObserved ||
		driver.callCount(nativeTargetServer) > 0 {
		return nativeACPCaseResult{reason: "the disabled plugin MCP target still executed"}
	}
	if !driver.hasNativeDenial(nativeTargetServer, false) {
		return nativeACPCaseResult{reason: "no authoritative native denial for the disabled target was observed"}
	}
	siblingSummary := sentinels.summary(nativeSiblingServer)
	if !siblingSummary.InitializeObserved || !siblingSummary.ToolsListObserved ||
		!siblingSummary.ToolCallObserved || siblingSummary.ToolCallRequestCount != 1 ||
		siblingSummary.ToolCallResultSentCount != 1 ||
		(driver.callCount(nativeSiblingServer) > 0 && !driver.hasSuccessfulSentinelCall(nativeSiblingServer)) {
		return nativeACPCaseResult{reason: "the allowed sibling MCP positive control did not complete"}
	}
	if len(driver.unexpected) != 0 {
		return nativeACPCaseResult{reason: "the native ACP client requested an unexpected tool or filesystem capability"}
	}
	if driver.captureFailed() {
		return nativeACPCaseResult{reason: "private ACP protocol capture could not be completed"}
	}
	return nativeACPCaseResult{passed: true}
}

func runNativeManagedPolicyCase(
	ctx context.Context,
	version string,
	baseArgs []string,
	env []string,
	paths nativeRunPaths,
	fixtures nativePluginFixture,
	policy nativePolicyFixture,
) (result nativeACPCaseResult) {
	recorder, err := newNativeRecorder(paths, "managed-policy")
	if err != nil {
		return nativeACPCaseResult{reason: "private ACP capture could not be created"}
	}
	defer func() { _ = recorder.Close() }()
	sentinels := newNativeTestSentinels(recorder)
	defer sentinels.close()
	if err := configurePluginMCPFixtures(fixtures, sentinels); err != nil {
		return nativeACPCaseResult{reason: "private plugin MCP fixture could not be written"}
	}

	driver, err := startNativeACPDriver(
		nativeACPArgs(version, paths, baseArgs, fixtures, sentinels, "", false, []string{
			nativeTargetServer,
			nativeSiblingServer,
		}),
		paths.workspace,
		env,
		recorder,
		sentinels,
		[]string{nativeTargetServer, nativeSiblingServer},
		[]string{nativeSiblingServer},
	)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeStartReason(err)}
	}
	defer func() { _ = driver.close() }()
	defer func() {
		captureNativeCaseEvidence(
			&result,
			driver,
			sentinels,
			[]string{policy.TargetServer, policy.AllowedServer},
			policy.TargetServer,
			true,
		)
	}()

	actualVersion, httpMCP, _, err := driver.initialize(ctx)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	if actualVersion != version || !httpMCP {
		return nativeACPCaseResult{reason: "managed-policy control did not start the exact ACP executable with HTTP MCP support"}
	}
	sessionID, err := driver.newSession(ctx, paths.workspace, nil)
	if err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	prompt := "Call each exact tool exactly once, in this order: " +
		strings.Join(nativeToolNames(policy.TargetServer, policy.AllowedServer), ", then ") +
		". These are harmless read-only sentinels. Do not call any other tool, read or write files, or run commands."
	if err := driver.prompt(ctx, sessionID, prompt); err != nil {
		return nativeACPCaseResult{reason: safeNativeACPReason(err)}
	}
	if driver.wasHarnessRejected(policy.TargetServer) {
		return nativeACPCaseResult{reason: "the ACP harness rejected the target permission, which is not managed-policy evidence"}
	}
	targetSummary := sentinels.summary(policy.TargetServer)
	if targetSummary.ToolCallRequestCount > 0 || targetSummary.ToolCallObserved ||
		driver.callCount(policy.TargetServer) > 0 {
		return nativeACPCaseResult{reason: "the managed-prohibited plugin MCP target still executed"}
	}
	if !driver.hasNativeDenial(policy.TargetServer, true) {
		return nativeACPCaseResult{reason: "no authoritative native managed-policy denial for the exact target identity was observed"}
	}
	allowedSummary := sentinels.summary(policy.AllowedServer)
	if !allowedSummary.InitializeObserved || !allowedSummary.ToolsListObserved ||
		!allowedSummary.ToolCallObserved || allowedSummary.ToolCallRequestCount != 1 ||
		allowedSummary.ToolCallResultSentCount != 1 ||
		(driver.callCount(policy.AllowedServer) > 0 && !driver.hasSuccessfulSentinelCall(policy.AllowedServer)) {
		return nativeACPCaseResult{reason: "the allowed managed-policy sibling positive control did not complete"}
	}
	if len(driver.unexpected) != 0 {
		return nativeACPCaseResult{reason: "the native ACP client requested an unexpected tool or filesystem capability"}
	}
	if driver.captureFailed() {
		return nativeACPCaseResult{reason: "private ACP protocol capture could not be completed"}
	}
	return nativeACPCaseResult{passed: true}
}

func safeNativeStartReason(err error) string {
	if err != nil && strings.Contains(err.Error(), "started") {
		return "the exact native package executable could not be started"
	}
	return "the native ACP driver could not be started"
}

func safeNativeACPReason(err error) string {
	if err == nil {
		return "native ACP operation failed"
	}
	switch {
	case strings.Contains(err.Error(), "authentication_required"):
		return "authentication_required"
	case strings.Contains(err.Error(), "timed out"):
		return "native ACP operation timed out"
	case strings.Contains(err.Error(), "ended before a response"):
		return "native ACP process ended before the requested response"
	default:
		return "native ACP operation failed; raw frames remain private"
	}
}
