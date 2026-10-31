package webapp

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var encodingQualityPattern = regexp.MustCompile(`^(0(\.[0-9]{0,3})?|1(\.0{0,3})?)$`)

func assetEncodingQualities(header string) (gzip, identity float64) {
	identity = 1
	qualities := make(map[string]float64)
	for _, entry := range strings.Split(header, ",") {
		parts := strings.Split(entry, ";")
		coding := strings.ToLower(strings.TrimSpace(parts[0]))
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, found := strings.Cut(parameter, "=")
			if !found || !strings.EqualFold(strings.TrimSpace(key), "q") || !encodingQualityPattern.MatchString(strings.TrimSpace(value)) {
				quality = 0
				break
			}
			quality, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
		}
		qualities[coding] = quality
	}
	gzip = qualities["*"]
	if value, exists := qualities["gzip"]; exists {
		gzip = value
	}
	if value, exists := qualities["identity"]; exists {
		identity = value
	} else if value, exists := qualities["*"]; exists && value == 0 {
		identity = 0
	}
	return gzip, identity
}

func varyAssetEncoding(header http.Header) {
	for _, line := range header.Values("Vary") {
		for _, value := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(value), "Accept-Encoding") || strings.TrimSpace(value) == "*" {
				return
			}
		}
	}
	header.Add("Vary", "Accept-Encoding")
}

type assetResponseWriter struct{ http.ResponseWriter }

func (w assetResponseWriter) WriteHeader(status int) {
	if status == http.StatusPreconditionFailed {
		w.Header().Del("Content-Length")
	}
	if status >= http.StatusBadRequest {
		w.Header().Del("Content-Encoding")
	}
	w.ResponseWriter.WriteHeader(status)
}
