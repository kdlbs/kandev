package plugins

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const usageCanonicalCursorPrefix = "usage-v1."

type usageCanonicalCursor struct {
	Offset     int    `json:"offset"`
	Generation string `json:"generation"`
	Query      string `json:"query"`
}

func usageCanonicalQueryKey(filter pluginsdk.SessionUsageFilter) (string, error) {
	encoded, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func decodeUsagePageCursor(cursor, query string, canonical bool) (int, string, error) {
	if !canonical {
		return pageOffset(cursor), "", nil
	}
	if cursor == "" {
		return 0, "", nil
	}
	payload, ok := strings.CutPrefix(cursor, usageCanonicalCursorPrefix)
	if !ok || len(payload) > 4096 {
		return 0, "", status.Error(codes.InvalidArgument, "invalid saved usage cursor")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(payload)
	var value usageCanonicalCursor
	if err != nil || json.Unmarshal(encoded, &value) != nil || value.Offset < 1 || value.Generation == "" || value.Query != query {
		return 0, "", status.Error(codes.InvalidArgument, "saved usage cursor does not match the query")
	}
	return value.Offset, value.Generation, nil
}

func encodeUsagePageCursor(offset int, generation, query string, canonical bool) (string, error) {
	if !canonical {
		return strconv.Itoa(offset), nil
	}
	if generation == "" {
		return "", status.Error(codes.FailedPrecondition, "saved usage publication is unavailable")
	}
	encoded, err := json.Marshal(usageCanonicalCursor{Offset: offset, Generation: generation, Query: query})
	if err != nil {
		return "", err
	}
	return usageCanonicalCursorPrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}
