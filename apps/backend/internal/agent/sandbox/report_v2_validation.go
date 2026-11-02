package sandbox

import (
	"slices"
	"time"
)

func validTypedReport(value *TypedReport, captured time.Time) bool {
	if value == nil {
		return true
	}
	if !validTypedReportHeader(value) {
		return false
	}

	if len(value.Sources) > 0 && !validApprovalDigest(value.ArtifactSetDigest) {
		return false
	}
	sources := make(map[string]bool, len(value.Sources))
	var total int64
	for _, source := range value.Sources {
		if !validTypedReportSource(source) || sources[source.ID] {
			return false
		}
		sources[source.ID] = true
		total += source.Bytes
	}
	if total > 32<<20 {
		return false
	}
	return validReportEvents(value.Records, sources, captured)
}

func validTypedReportSource(source TypedReportSource) bool {
	return validApprovalDigest(source.ID) && reportText(source.Schema, 128, false) && validApprovalDigest(source.Digest) && validApprovalDigest(source.Identity) && source.Bytes >= 0 && source.Bytes <= 32<<20
}

func validReportCoverage(values []ReportCoverage) bool {
	if len(values) != 4 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !slices.Contains([]string{reportClassFilesystem, reportClassNetwork, reportClassCommand, reportClassEndpoint}, value.Kind) || seen[value.Kind] || !slices.Contains([]string{"supported", reportIntegrityPartial, reportStatusUnsupported, reportStatusUnavailable}, value.Coverage) {
			return false
		}
		seen[value.Kind] = true
	}
	return true
}

func validReportEvents(records []ReportEvent, sources map[string]bool, captured time.Time) bool {
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if !validReportEvent(record, captured) || !sources[record.SourceID] || seen[record.ID] {
			return false
		}
		seen[record.ID] = true
	}
	return true
}

func validReportEvent(value ReportEvent, captured time.Time) bool {
	if !validApprovalDigest(value.ID) || !reportText(value.ProviderRecordID, 256, false) || value.Sequence >= 65536 || value.OccurredAt.UnixMilli() <= 0 || value.OccurredAt.After(captured.Add(5*time.Second)) {
		return false
	}
	if !slices.Contains([]string{reportDecisionAllowed, ApprovalDenied, "approval_requested", "approval_granted", "approval_denied", "error"}, value.Decision) || !validReportReason(value.ReasonCode) {
		return false
	}
	if !slices.Contains([]string{"", reportIncompleteIdentity, "not_denied", "unsupported_policy"}, value.LearningBlocker) || !value.ExactTarget && value.LearningBlocker != reportIncompleteIdentity {
		return false
	}
	return value.Operation.valid(value.Kind, value.ExactTarget)
}

func validReportReason(value string) bool {
	return slices.Contains([]string{"", "permission_denied", "approval_denied", "approval_timeout", "approval_granted", "authentication_failed", "upstream_unavailable", "unsupported_protocol", "command_failed", reportReasonOther}, value)
}

func typedReportSourcesMatch(data ReportData) bool {
	if data.V2 == nil {
		return true
	}
	if len(data.V2.Sources) != len(data.Sources) {
		return false
	}
	sources := make(map[string]ReportSource, len(data.Sources))
	for _, source := range data.Sources {
		sources[source.ID] = source
	}
	for _, source := range data.V2.Sources {
		legacy, ok := sources[source.ID]
		if !ok || source.Schema != legacy.Schema || source.Digest != legacy.Digest {
			return false
		}
	}
	return true
}

func validTypedReportHeader(value *TypedReport) bool {
	if value.SchemaVersion != 2 || !slices.Contains([]string{auditStateUnchecked, auditStateValid, auditStateInvalid}, value.Integrity) || !slices.Contains([]string{"unknown", "live", reportFinalizationFinal, reportFinalizationInterrupted}, value.Finalization) {
		return false
	}
	if len(value.Sources) > 32 || len(value.Records) > 1000 || !validReportCoverage(value.Coverage) {
		return false
	}
	return len(value.Sources) != 0 || value.ArtifactSetDigest == "" && value.Integrity == auditStateUnchecked && len(value.Records) == 0 && value.Finalization != reportFinalizationFinal
}
