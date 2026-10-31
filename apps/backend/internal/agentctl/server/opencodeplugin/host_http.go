package opencodeplugin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

var errRequestTooLarge = errors.New("request body exceeds the accepted limit")

func decodeLimitedJSON(request *http.Request, target any, limit int64) error {
	body, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return errRequestTooLarge
	}
	return decodeStrict(body, target)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]string{"error": code})
}

func parseRevision(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}
