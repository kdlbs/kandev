package nomad

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

const maxJobResponseBytes = 4 << 20
const maxJobResponseDepth = 32

const (
	pathTaskGroup       = "$.TaskGroups[]"
	pathTask            = "$.TaskGroups[].Tasks[]"
	pathDynamicPorts    = "$.TaskGroups[].Networks[].DynamicPorts[]"
	keyAffinities       = "Affinities"
	reviewedDefaultName = "default"
)

// ValidateNativeJobResponse checks a complete Nomad GET /v1/job response
// against the locally rebuilt fixed-policy job. Unknown JSON keys, duplicate
// keys, unreviewed defaults, and all behavior differences fail closed.
func ValidateNativeJobResponse(raw []byte, expected Job) error {
	if len(raw) == 0 || len(raw) > maxJobResponseBytes {
		return fmt.Errorf("nomad job response has invalid size")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	actual, err := decodeExactJSON(raw)
	if err != nil {
		return fmt.Errorf("decode complete Nomad job response: %w", err)
	}
	if err := validateJobFieldTree(actual, "$", 0); err != nil {
		return err
	}
	actualNormalized, err := normalizeJobResponse(actual, "$", nil)
	if err != nil {
		return err
	}
	expectedRaw, err := json.Marshal(expected)
	if err != nil {
		return fmt.Errorf("encode expected Nomad job: %w", err)
	}
	expectedValue, err := decodeExactJSON(expectedRaw)
	if err != nil {
		return fmt.Errorf("decode expected Nomad job: %w", err)
	}
	expectedNormalized, err := normalizeJobResponse(expectedValue, "$", nil)
	if err != nil {
		return err
	}
	actualCanonical, err := json.Marshal(actualNormalized)
	if err != nil {
		return fmt.Errorf("canonicalize Nomad job response: %w", err)
	}
	expectedCanonical, err := json.Marshal(expectedNormalized)
	if err != nil {
		return fmt.Errorf("canonicalize expected Nomad job: %w", err)
	}
	if !bytes.Equal(actualCanonical, expectedCanonical) {
		path := firstJSONDifference(actualNormalized, expectedNormalized, "$")
		return fmt.Errorf("nomad job response differs from the exact immutable native policy at %s", path)
	}
	digest, err := NativeJobSpecDigest(expected)
	if err != nil || expected.Meta["kandev.native_job_spec_sha256"] != digest {
		return fmt.Errorf("expected Nomad job does not carry its matching immutable policy digest")
	}
	return nil
}

// firstJSONDifference returns a deterministic field path without including any
// values. Job JSON may contain environment values and bootstrap paths, so
// reconciliation errors must identify the mismatch without echoing content.
func firstJSONDifference(actual, expected any, path string) string {
	actualObject, actualIsObject := actual.(map[string]any)
	expectedObject, expectedIsObject := expected.(map[string]any)
	if actualIsObject && expectedIsObject {
		return firstObjectDifference(actualObject, expectedObject, path)
	}
	actualArray, actualIsArray := actual.([]any)
	expectedArray, expectedIsArray := expected.([]any)
	if actualIsArray && expectedIsArray {
		return firstArrayDifference(actualArray, expectedArray, path)
	}
	if !reflect.DeepEqual(actual, expected) {
		return path
	}
	return ""
}

func firstObjectDifference(actualObject, expectedObject map[string]any, path string) string {
	keys := make([]string, 0, len(actualObject)+len(expectedObject))
	seen := make(map[string]struct{}, len(actualObject)+len(expectedObject))
	for key := range actualObject {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range expectedObject {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		actualValue, actualOK := actualObject[key]
		expectedValue, expectedOK := expectedObject[key]
		childPath := path + "." + key
		if !actualOK || !expectedOK {
			return childPath
		}
		if difference := firstJSONDifference(actualValue, expectedValue, childPath); difference != "" {
			return difference
		}
	}
	return ""
}

func firstArrayDifference(actualArray, expectedArray []any, path string) string {
	if len(actualArray) != len(expectedArray) {
		return path
	}
	for index := range actualArray {
		if difference := firstJSONDifference(actualArray[index], expectedArray[index], fmt.Sprintf("%s[%d]", path, index)); difference != "" {
			return difference
		}
	}
	return ""
}

func decodeExactJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("nomad job response must be an object")
	}
	return value, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("nomad job response contains trailing JSON data")
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxJobResponseDepth {
		return fmt.Errorf("nomad job response exceeds maximum JSON depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read Nomad job response: %w", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeJSONObject(decoder, depth)
	case '[':
		return consumeJSONArray(decoder, depth)
	default:
		return fmt.Errorf("nomad job response contains an unexpected JSON delimiter")
	}
}

func consumeJSONObject(decoder *json.Decoder, depth int) error {
	seen := map[string]struct{}{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("read Nomad job object key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("nomad job object contains a non-string key")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("nomad job response contains duplicate JSON key %q", key)
		}
		seen[key] = struct{}{}
		if err := consumeJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("close Nomad job object: %w", err)
	}
	return nil
}

func consumeJSONArray(decoder *json.Decoder, depth int) error {
	for decoder.More() {
		if err := consumeJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("close Nomad job array: %w", err)
	}
	return nil
}

var jobResponseFields = map[string]map[string]struct{}{
	"$":                                           fields("ID Name Type Priority AllAtOnce NodePool Stop ParentID Dispatched Payload Namespace Region Datacenters Meta Constraints Affinities Spreads TaskGroups Update Multiregion Periodic ParameterizedJob DispatchIdempotencyToken ConsulNamespace VaultNamespace NomadTokenID Status StatusDescription Stable Version SubmitTime CreateIndex ModifyIndex JobModifyIndex UI VersionTag"),
	"$.Constraints[]":                             fields("LTarget Operand RTarget"),
	"$.Affinities[]":                              fields("LTarget Operand RTarget Weight"),
	"$.Spreads[]":                                 fields("Attribute Weight SpreadTarget"),
	"$.TaskGroups[].Constraints[]":                fields("LTarget Operand RTarget"),
	"$.TaskGroups[].Tasks[].Constraints[]":        fields("LTarget Operand RTarget"),
	pathTaskGroup:                                 fields("Name Count Update Migrate Constraints Scaling RestartPolicy Disconnect Tasks EphemeralDisk Meta ReschedulePolicy Affinities Spreads Networks Consul Services Volumes HasPerAllocVolumes ShutdownDelay MaxRunDuration StopAfterClientDisconnect MaxClientDisconnect PreventRescheduleOnLost"),
	pathTask:                                      fields("Name Driver User Config Env Services Vault Secrets Consul Templates Constraints Affinities Resources RestartPolicy DispatchPayload Lifecycle Meta LogConfig Artifacts Leader ShutdownDelay VolumeMounts ScalingPolicies KillSignal Kind CSIPluginConfig Identity Identities Actions Schedule KillTimeout"),
	"$.TaskGroups[].RestartPolicy":                fields("Attempts Interval Delay Mode RenderTemplates"),
	"$.TaskGroups[].ReschedulePolicy":             fields("Attempts Interval Delay DelayFunction MaxDelay Unlimited"),
	"$.TaskGroups[].Disconnect":                   fields("Replace Reconcile LostAfter StopOnClientAfter"),
	"$.TaskGroups[].EphemeralDisk":                fields("SizeMB Sticky Migrate"),
	"$.Update":                                    fields("AutoPromote AutoRevert Canary HealthCheck HealthyDeadline MaxParallel MinHealthyTime ProgressDeadline Stagger"),
	"$.TaskGroups[].Networks[]":                   fields("Mode Device CIDR IP Hostname MBits DNS ReservedPorts DynamicPorts CNI"),
	"$.TaskGroups[].Networks[].ReservedPorts[]":   fields("Label Value To HostNetwork IgnoreCollision"),
	pathDynamicPorts:                              fields("Label Value To HostNetwork IgnoreCollision"),
	"$.TaskGroups[].Tasks[].Resources":            fields("CPU Cores MemoryMB MemoryMaxMB DiskMB IOPS Networks Devices NUMA SecretsMB"),
	"$.TaskGroups[].Tasks[].RestartPolicy":        fields("Attempts Interval Delay Mode RenderTemplates"),
	"$.TaskGroups[].Tasks[].LogConfig":            fields("MaxFiles MaxFileSizeMB Disabled"),
	"$.TaskGroups[].Tasks[].Identity":             fields("Name Audience ChangeMode ChangeSignal Env File Filepath ServiceName TTL ExtraClaims"),
	"$.TaskGroups[].Tasks[].Identities[]":         fields("Name Audience ChangeMode ChangeSignal Env File Filepath ServiceName TTL ExtraClaims"),
	"$.TaskGroups[].Networks[].DNS":               fields("Servers Searches Options"),
	"$.TaskGroups[].Tasks[].Resources.Networks[]": fields("Mode Device CIDR IP Hostname MBits DNS ReservedPorts DynamicPorts CNI"),
	"$.TaskGroups[].Tasks[].Resources.Networks[].DynamicPorts[]":  fields("Label Value To HostNetwork IgnoreCollision"),
	"$.TaskGroups[].Tasks[].Resources.Networks[].ReservedPorts[]": fields("Label Value To HostNetwork IgnoreCollision"),
	"$.TaskGroups[].Tasks[].Resources.Networks[].DNS":             fields("Servers Searches Options"),
	"$.TaskGroups[].Tasks[].Resources.NUMA":                       fields("Affinity"),
	"$.TaskGroups[].Tasks[].Resources.Devices[]":                  fields("Name Count Constraints"),
	"$.TaskGroups[].Tasks[].Resources.Devices[].Constraints[]":    fields("LTarget Operand RTarget"),
}

var opaqueJobMapPaths = map[string]struct{}{
	"$.Meta":                        {},
	"$.TaskGroups[].Meta":           {},
	"$.TaskGroups[].Tasks[].Config": {},
	"$.TaskGroups[].Tasks[].Env":    {},
}

func fields(list string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, field := range strings.Fields(list) {
		result[field] = struct{}{}
	}
	return result
}

func validateJobFieldTree(value any, path string, depth int) error {
	if depth > maxJobResponseDepth {
		return fmt.Errorf("nomad job response exceeds maximum schema depth")
	}
	switch node := value.(type) {
	case map[string]any:
		return validateJobObject(node, path, depth)
	case []any:
		itemPath := path + "[]"
		for _, child := range node {
			if err := validateJobFieldTree(child, itemPath, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateJobObject(node map[string]any, path string, depth int) error {
	allowed, constrained := jobResponseFields[path]
	_, opaque := opaqueJobMapPaths[path]
	if !constrained && !opaque {
		return fmt.Errorf("nomad job response contains an unreviewed object at %s", path)
	}
	if constrained {
		for key := range node {
			if _, ok := allowed[key]; !ok {
				return fmt.Errorf("nomad job response contains unknown field %s.%s", path, key)
			}
		}
	}
	for key, child := range node {
		switch child.(type) {
		case map[string]any, []any:
			if err := validateJobFieldTree(child, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeJobResponse(value any, path string, inheritedRestart any) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		return normalizeJobObject(node, path, inheritedRestart)
	case []any:
		result := make([]any, 0, len(node))
		for _, child := range node {
			normalized, err := normalizeJobResponse(child, path+"[]", inheritedRestart)
			if err != nil {
				return nil, err
			}
			result = append(result, normalized)
		}
		return result, nil
	default:
		return value, nil
	}
}

func normalizeJobObject(node map[string]any, path string, inheritedRestart any) (any, error) {
	result := make(map[string]any, len(node))
	var groupRestart any
	if path == pathTaskGroup {
		groupRestart = node["RestartPolicy"]
	}
	for key, child := range node {
		childPath := path + "." + key
		if isJobObservation(path, key, child) || isReviewedJobDefault(path, key, child) {
			continue
		}
		if path == pathTask && key == "RestartPolicy" && equalRestartPolicy(child, inheritedRestart) {
			continue
		}
		nextRestart := inheritedRestart
		if path == pathTaskGroup {
			nextRestart = groupRestart
		}
		normalized, err := normalizeJobResponse(child, childPath, nextRestart)
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}
	// Nomad 2.0.7 omits zero-valued dynamic-port Value/To fields in its
	// persisted Job JSON even though the create request explicitly sends
	// both as zero. Only this exact path and absent-field case are reviewed.
	if path == pathDynamicPorts {
		for _, key := range []string{"Value", "To"} {
			if _, exists := node[key]; !exists {
				result[key] = json.Number("0")
			}
		}
	}
	return result, nil
}

func isJobObservation(parent, key string, value any) bool {
	if parent != "$" {
		return false
	}
	switch key {
	case "Status", "StatusDescription", "Stable", "Version", "SubmitTime", "CreateIndex", "ModifyIndex", "JobModifyIndex", "NomadTokenID":
		return true
	default:
		return false
	}
}

func isReviewedJobDefault(parent, key string, value any) bool {
	switch parent {
	case "$":
		return isReviewedRootDefault(key, value)
	case pathTaskGroup:
		return isReviewedGroupDefault(key, value)
	case pathTask:
		return isReviewedTaskDefault(key, value)
	case "$.TaskGroups[].Tasks[].Resources":
		return isReviewedResourcesDefault(key, value)
	}
	return isReviewedNestedDefault(parent, key, value)
}

func isReviewedNestedDefault(parent, key string, value any) bool {
	switch parent {
	case "$.TaskGroups[].RestartPolicy", "$.TaskGroups[].Tasks[].RestartPolicy":
		return key == "RenderTemplates" && value == false
	case pathDynamicPorts:
		return (key == "HostNetwork" && value == reviewedDefaultName) || (key == "IgnoreCollision" && value == false)
	case "$.Spreads[]":
		return key == "SpreadTarget" && (value == nil || isEmptyMapOrArray(value))
	case "$.TaskGroups[].Disconnect":
		return (key == "LostAfter" && value == json.Number("0")) || (key == "StopOnClientAfter" && value == nil)
	}
	return false
}

func isReviewedRootDefault(key string, value any) bool {
	switch key {
	case "AllAtOnce", "Stop", "Dispatched":
		return value == false
	case "ParentID", "ConsulNamespace", "VaultNamespace", "DispatchIdempotencyToken":
		return value == ""
	case "Payload":
		return value == nil || reflect.DeepEqual(value, []any{})
	case "Update":
		return value == nil || isExactObject(value, map[string]any{
			"AutoPromote": false, "AutoRevert": false, "Canary": json.Number("0"),
			"HealthCheck": "", "HealthyDeadline": json.Number("0"), "MaxParallel": json.Number("0"),
			"MinHealthyTime": json.Number("0"), "ProgressDeadline": json.Number("0"), "Stagger": json.Number("0"),
		})
	case keyAffinities, "Spreads", "Multiregion", "Periodic", "ParameterizedJob", "UI", "VersionTag":
		return value == nil || isEmptyMapOrArray(value)
	}
	return false
}

func isReviewedGroupDefault(key string, value any) bool {
	switch key {
	case "HasPerAllocVolumes", "PreventRescheduleOnLost", "RenderTemplates":
		return value == false
	case "Meta", "Constraints", keyAffinities, "Spreads", "Services", "Volumes":
		return value == nil || isEmptyMapOrArray(value)
	case "EphemeralDisk":
		return isExactObject(value, map[string]any{"SizeMB": json.Number("300"), "Sticky": false, "Migrate": false})
	case "Update", "Migrate", "Scaling", "Consul", "ShutdownDelay", "MaxRunDuration", "StopAfterClientDisconnect", "MaxClientDisconnect":
		return value == nil
	}
	return false
}

func isReviewedTaskDefault(key string, value any) bool {
	switch key {
	case "User", "KillSignal", "Kind":
		return value == ""
	case "Leader":
		return value == false
	case "Services", "Secrets", "Constraints", keyAffinities, "Templates", "Artifacts", "VolumeMounts", "ScalingPolicies", "Identities", "Actions", "Meta":
		return value == nil || isEmptyMapOrArray(value)
	case "Lifecycle", "Vault", "Consul", "DispatchPayload", "CSIPluginConfig", "Schedule":
		return value == nil
	case "ShutdownDelay":
		return value == json.Number("0")
	case "Identity":
		return isDefaultWorkloadIdentity(value)
	case "LogConfig":
		return isExactObject(value, map[string]any{"MaxFiles": json.Number("10"), "MaxFileSizeMB": json.Number("10"), "Disabled": false})
	}
	return false
}

func isReviewedResourcesDefault(key string, value any) bool {
	switch key {
	case "Cores", "DiskMB", "IOPS", "MemoryMaxMB", "SecretsMB":
		return value == json.Number("0")
	case "Devices", "Networks":
		return value == nil || isEmptyMapOrArray(value)
	case "NUMA":
		return value == nil
	}
	return false
}

func equalRestartPolicy(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	left, okLeft := a.(map[string]any)
	right, okRight := b.(map[string]any)
	if !okLeft || !okRight {
		return false
	}
	leftCopy := cloneJSONMap(left)
	rightCopy := cloneJSONMap(right)
	delete(leftCopy, "RenderTemplates")
	delete(rightCopy, "RenderTemplates")
	return reflect.DeepEqual(leftCopy, rightCopy) && (left["RenderTemplates"] == nil || left["RenderTemplates"] == false) && (right["RenderTemplates"] == nil || right["RenderTemplates"] == false)
}

func isDefaultWorkloadIdentity(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	allowed := map[string]any{
		"Name": reviewedDefaultName, "Audience": []any{"nomadproject.io"},
		"ChangeMode": "", "ChangeSignal": "", "Env": false, "File": false,
		"Filepath": "", "ServiceName": "", "TTL": json.Number("0"), "ExtraClaims": nil,
	}
	for key, item := range object {
		if expected, exists := allowed[key]; !exists || !reflect.DeepEqual(item, expected) {
			return false
		}
	}
	return reflect.DeepEqual(object["Name"], reviewedDefaultName)
}

func isEmptyMapOrArray(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		return len(node) == 0
	case []any:
		return len(node) == 0
	default:
		return false
	}
}

func isExactObject(value any, expected map[string]any) bool {
	actual, ok := value.(map[string]any)
	if !ok || len(actual) != len(expected) {
		return false
	}
	for key, want := range expected {
		if !reflect.DeepEqual(actual[key], want) {
			return false
		}
	}
	return true
}

func cloneJSONMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
