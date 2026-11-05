package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (o ReportOperation) valid(kind string, exact bool) bool {
	count := 0
	for _, present := range []bool{o.Filesystem != nil, o.TCP != nil, o.Command != nil, o.Endpoint != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return false
	}
	switch kind {
	case reportClassFilesystem:
		return o.Filesystem != nil && reportAbsolutePath(o.Filesystem.Path) && slices.Contains([]string{filesystemRead, filesystemWrite, "read_write"}, o.Filesystem.Access)
	case reportClassNetwork:
		return validReportTCP(o.TCP, exact)
	case reportClassCommand:
		return !exact && o.Command != nil && reportText(o.Command.ID, 128, false) && !strings.ContainsAny(o.Command.ID, "/\\ ") && reportText(o.Command.Caller, 128, false)
	case reportClassEndpoint:
		return validReportEndpoint(o.Endpoint, exact)
	default:
		return false
	}
}

func validReportTCP(value *ReportTCP, exact bool) bool {
	if value == nil || value.Port > 65535 || value.Transport != reportTransportTCP || exact && (value.Host == "" || value.Port == 0) {
		return false
	}
	if value.Host == "" {
		return true
	}
	port := uint16(max(value.Port, 1))
	grants, err := contract.Canonicalize(contract.Grants{Network: []contract.NetworkGrant{{Host: value.Host, Port: port, Protocol: reportTransportTCP}}}, nil)
	return err == nil && len(grants.Network) == 1 && grants.Network[0].Host == value.Host
}

func validReportEndpoint(value *ReportEndpoint, exact bool) bool {
	if value == nil || !slices.Contains([]string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}, value.Method) || !reportText(value.RouteID, 128, true) {
		return false
	}
	if value.Path != "" && (!reportAbsolutePath(value.Path) || strings.ContainsAny(value.Path, "%?#")) {
		return false
	}
	if exact && (value.RouteID == "" || value.Path == "" || !strings.HasPrefix(value.Origin, "https://")) {
		return false
	}
	if value.Origin == "" {
		return !exact
	}
	origin, err := url.Parse(value.Origin)
	return err == nil && reportOriginShape(origin)
}

func reportOriginShape(origin *url.URL) bool {
	if origin == nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.User != nil || origin.Path != "" || origin.RawPath != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.Opaque != "" {
		return false
	}
	port := uint32(443)
	if origin.Scheme == "http" {
		port = 80
	}
	if origin.Port() != "" {
		value, err := strconv.ParseUint(origin.Port(), 10, 16)
		if err != nil {
			return false
		}
		port = uint32(value)
	}

	return port > 0 && validReportTCP(&ReportTCP{Host: origin.Hostname(), Port: port, Transport: reportTransportTCP}, true)
}

func reportAbsolutePath(value string) bool {
	return reportText(value, 4096, false) && path.IsAbs(value) && path.Clean(value) == value && !strings.Contains(value, "\\")
}

func (o *ReportOperation) UnmarshalJSON(data []byte) error {
	type plain ReportOperation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value plain
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return contract.ErrInvalidPolicy
	}
	*o = ReportOperation(value)
	return nil
}
