package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func reportText(value string, maximum int, optional bool) bool {
	return (optional || value != "") && len(value) <= maximum && !strings.ContainsFunc(value, unicode.IsControl)
}

func reportIntegrity(value string) int {
	switch value {
	case reportIntegrityComplete:
		return 2
	case "partial":
		return 1
	case reportIntegrityUnverified:
		return 0
	default:
		return -1
	}
}

func validReportEnvelope(data ReportData) bool {
	if !slices.Contains([]string{"available", "unavailable", "unsupported"}, data.Status) ||
		reportIntegrity(data.Integrity) < 0 || len(data.Sources) > 32 || len(data.Records) > 1000 || len(data.SupportedClasses) > 3 {
		return false
	}
	if data.Status == "unsupported" && (len(data.Sources) != 0 || len(data.Records) != 0) {
		return false
	}
	if !validReportClasses(data.SupportedClasses) || !validReportCompleteness(data) {
		return false
	}
	return true
}

func validReportData(data ReportData, captured time.Time) bool {
	if !validReportEnvelope(data) {
		return false
	}
	sources := make(map[string]bool, len(data.Sources))
	for _, source := range data.Sources {
		if !validReportSource(source) || sources[source.ID] || reportIntegrity(data.Integrity) > reportIntegrity(source.Integrity) {
			return false
		}
		sources[source.ID] = true
	}
	seen := make(map[string]bool, len(data.Records))
	for _, record := range data.Records {
		if !validDenialRecord(record, captured) || !sources[record.SourceID] || seen[record.ID] || !slices.Contains(data.SupportedClasses, record.Kind) {
			return false
		}
		seen[record.ID] = true
	}
	return true
}

func validReportClasses(classes []string) bool {
	seen := map[string]bool{}
	for _, class := range classes {
		if seen[class] || !slices.Contains([]string{"filesystem", "network", "command"}, class) {
			return false
		}
		seen[class] = true
	}
	return true
}

func validReportCompleteness(data ReportData) bool {
	if len(data.Sources) == 0 && data.Integrity != reportIntegrityUnverified {
		return false
	}
	if !data.Complete {
		return true
	}
	if data.Status != "available" || data.Integrity != reportIntegrityComplete || len(data.Sources) == 0 {
		return false
	}
	for _, source := range data.Sources {
		if !source.Complete || source.NextCursor != "" {
			return false
		}
	}
	return true
}

func validReportSource(source ReportSource) bool {
	return reportText(source.ID, 256, false) && reportText(source.Schema, 128, false) && validApprovalDigest(source.Digest) &&
		reportText(source.Cursor, 1024, true) && reportText(source.NextCursor, 1024, true) && reportIntegrity(source.Integrity) >= 0 &&
		(!source.Complete || source.Integrity == reportIntegrityComplete)
}

func validDenialRecord(record DenialRecord, captured time.Time) bool {
	if !reportText(record.ID, 256, false) || !reportText(record.ProviderRecordID, 256, false) ||
		!reportText(record.Target, 4096, false) || record.OccurredAt.UnixMilli() < 0 || record.OccurredAt.After(captured.Add(5*time.Second)) {
		return false
	}
	switch record.Kind {
	case "filesystem":
		return record.Operation == filesystemRead || record.Operation == filesystemWrite
	case "network":
		return record.Operation == "connect"
	case "command":
		return record.Operation == "execute"
	default:
		return false
	}
}

func reportDigest(report Report) (string, error) {
	report.Digest = ""
	raw, err := json.Marshal(report)
	if err != nil || len(raw) > 256<<10 {
		return "", contract.ErrInvalidPolicy
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func reportMatchesAttempt(report *Report, attempt *Attempt) bool {
	resolved := attempt.Snapshot.Resolved
	return report.AttemptID == attempt.ID && report.SessionID == attempt.Snapshot.SessionID &&
		report.RevisionID == resolved.Revision.ID && report.PolicyDigest == resolved.Revision.Digest &&
		report.CeilingDigest == resolved.Ceiling.Digest && report.Provider == attempt.Snapshot.Provider
}

func validStoredReport(report *Report, version int64, finalized int) bool {
	digest, err := reportDigest(*report)
	return err == nil && digest == report.Digest && report.SchemaVersion == 1 &&
		report.Version == version && version > 0 && (finalized == 0 || finalized == 1) &&
		report.Data.Finalized == (finalized == 1) && !report.CapturedAt.IsZero()
}
