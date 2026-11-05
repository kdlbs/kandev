package launcher

import (
	"encoding/json"
	"fmt"
	"io"
)

type versionJSON struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func printVersionJSON(stdout, stderr io.Writer, build BuildInfo) int {
	commit, buildTime := build.Commit, build.BuildTime
	if commit == "" {
		commit = "unknown"
	}
	if buildTime == "" {
		buildTime = "unknown"
	}
	result := versionJSON{Version: build.Version, Commit: commit, BuildTime: buildTime}
	if err := json.NewEncoder(versionJSONWriter{stdout}).Encode(result); err != nil {
		_, _ = fmt.Fprintf(stderr, "[kandev] failed to write version output: %v\n", err)
		return 1
	}
	return 0
}

type versionJSONWriter struct{ io.Writer }

func (w versionJSONWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}
