package mcp

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	maxRichOutputCSVBytes  = 256 * 1024
	maxRichOutputCSVRows   = 100
	maxRichOutputChartText = 120
)

type richOutputCSVSeriesInput struct {
	Column string `json:"column"`
	Label  string `json:"label,omitempty"`
}

type richOutputCSVSourceInput struct {
	Path    string                     `json:"path"`
	Repo    string                     `json:"repo,omitempty"`
	XColumn string                     `json:"x_column"`
	Series  []richOutputCSVSeriesInput `json:"series"`
}

type richOutputCSVBlockInput struct {
	CSV *richOutputCSVSourceInput `json:"csv,omitempty"`
}

type richOutputCSVPresentationInput struct {
	Blocks []richOutputCSVBlockInput `json:"blocks"`
}

type richOutputResolvedSeries struct {
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

type richOutputResolvedChart struct {
	BlockIndex int                        `json:"block_index"`
	Labels     []string                   `json:"labels"`
	Series     []richOutputResolvedSeries `json:"series"`
}

type richOutputCSVSnapshot struct {
	Version        int                       `json:"version"`
	ResolvedCharts []richOutputResolvedChart `json:"resolved_charts"`
}

func (s *Server) resolveRichOutputCSV(
	ctx context.Context,
	args map[string]interface{},
) (*richOutputCSVSnapshot, error) {
	presentation, err := decodeRichOutputCSVInput(args)
	if err != nil {
		return nil, err
	}
	snapshot := &richOutputCSVSnapshot{Version: 1}
	cache := make(map[string]*streams.FileContentResponse)
	for blockIndex, block := range presentation.Blocks {
		if block.CSV == nil {
			continue
		}
		response, readErr := s.readRichOutputCSV(ctx, block.CSV, cache)
		if readErr != nil {
			return nil, readErr
		}
		resolved, parseErr := parseRichOutputCSV(*block.CSV, response.Content, blockIndex)
		if parseErr != nil {
			return nil, fmt.Errorf("CSV source %s: %w", block.CSV.Path, parseErr)
		}
		snapshot.ResolvedCharts = append(snapshot.ResolvedCharts, resolved)
	}
	if len(snapshot.ResolvedCharts) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || len(encoded) > maxRichOutputPayloadBytes {
		return nil, fmt.Errorf("normalized CSV chart snapshot exceeds 64 KiB")
	}
	return snapshot, nil
}

func decodeRichOutputCSVInput(args map[string]interface{}) (richOutputCSVPresentationInput, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return richOutputCSVPresentationInput{}, fmt.Errorf("rich output payload is invalid")
	}
	var presentation richOutputCSVPresentationInput
	if err := json.Unmarshal(encoded, &presentation); err != nil {
		return richOutputCSVPresentationInput{}, fmt.Errorf("rich output payload is invalid")
	}
	return presentation, nil
}

func (s *Server) readRichOutputCSV(
	ctx context.Context,
	source *richOutputCSVSourceInput,
	cache map[string]*streams.FileContentResponse,
) (*streams.FileContentResponse, error) {
	cacheKey := source.Repo + "\x00" + source.Path
	if cached := cache[cacheKey]; cached != nil {
		return cached, nil
	}
	payload := map[string]interface{}{richOutputFieldSessionID: s.sessionID, richOutputFieldPath: source.Path}
	if source.Repo != "" {
		payload["repo"] = source.Repo
	}
	var response streams.FileContentResponse
	if err := s.backend.RequestPayload(ctx, ws.ActionWorkspaceFileContentGet, payload, &response); err != nil {
		return nil, fmt.Errorf("could not read CSV source %s: %w", source.Path, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("could not read CSV source %s: %s", source.Path, response.Error)
	}
	if response.IsBinary || !utf8.ValidString(response.Content) {
		return nil, fmt.Errorf("CSV source must be UTF-8 text")
	}
	if response.Size > maxRichOutputCSVBytes || len(response.Content) > maxRichOutputCSVBytes {
		return nil, fmt.Errorf("CSV source exceeds 256 KiB")
	}
	cache[cacheKey] = &response
	return &response, nil
}

func parseRichOutputCSV(
	source richOutputCSVSourceInput,
	content string,
	blockIndex int,
) (richOutputResolvedChart, error) {
	reader := csv.NewReader(strings.NewReader(content))
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return richOutputResolvedChart{}, fmt.Errorf("CSV source must contain a header row")
	}
	if err != nil {
		return richOutputResolvedChart{}, fmt.Errorf("CSV header is invalid: %w", err)
	}
	headerIndexes, err := indexRichOutputCSVHeaders(header)
	if err != nil {
		return richOutputResolvedChart{}, err
	}
	xIndex, seriesIndexes, err := resolveRichOutputCSVColumns(source, headerIndexes)
	if err != nil {
		return richOutputResolvedChart{}, err
	}
	reader.FieldsPerRecord = len(header)
	resolved := newRichOutputResolvedChart(source, blockIndex)
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return richOutputResolvedChart{}, fmt.Errorf("CSV row %d is invalid: %w", rowNumber, readErr)
		}
		if len(resolved.Labels) == maxRichOutputCSVRows {
			return richOutputResolvedChart{}, fmt.Errorf("CSV source must contain at most 100 data rows")
		}
		if err := appendRichOutputCSVRow(&resolved, source, record, rowNumber, xIndex, seriesIndexes); err != nil {
			return richOutputResolvedChart{}, err
		}
	}
	if len(resolved.Labels) == 0 {
		return richOutputResolvedChart{}, fmt.Errorf("CSV source must contain 1 to 100 data rows")
	}
	return resolved, nil
}

func indexRichOutputCSVHeaders(header []string) (map[string]int, error) {
	indexes := make(map[string]int, len(header))
	for index, rawName := range header {
		name := strings.TrimSpace(strings.TrimPrefix(rawName, "\ufeff"))
		if name == "" {
			return nil, fmt.Errorf("CSV header %d must not be blank", index+1)
		}
		if _, exists := indexes[name]; exists {
			return nil, fmt.Errorf("CSV header %q is duplicated", name)
		}
		indexes[name] = index
	}
	return indexes, nil
}

func resolveRichOutputCSVColumns(
	source richOutputCSVSourceInput,
	headers map[string]int,
) (int, []int, error) {
	xIndex, ok := headers[source.XColumn]
	if !ok {
		return 0, nil, fmt.Errorf("CSV column %q was not found", source.XColumn)
	}
	seriesIndexes := make([]int, len(source.Series))
	for index, series := range source.Series {
		columnIndex, exists := headers[series.Column]
		if !exists {
			return 0, nil, fmt.Errorf("CSV column %q was not found", series.Column)
		}
		seriesIndexes[index] = columnIndex
	}
	return xIndex, seriesIndexes, nil
}

func newRichOutputResolvedChart(
	source richOutputCSVSourceInput,
	blockIndex int,
) richOutputResolvedChart {
	resolved := richOutputResolvedChart{BlockIndex: blockIndex}
	for _, input := range source.Series {
		label := input.Label
		if label == "" {
			label = input.Column
		}
		resolved.Series = append(resolved.Series, richOutputResolvedSeries{Label: label})
	}
	return resolved
}

func appendRichOutputCSVRow(
	resolved *richOutputResolvedChart,
	source richOutputCSVSourceInput,
	record []string,
	rowNumber, xIndex int,
	seriesIndexes []int,
) error {
	label := strings.TrimSpace(record[xIndex])
	if label == "" {
		return fmt.Errorf("CSV row %d column %q must not be blank", rowNumber, source.XColumn)
	}
	if utf8.RuneCountInString(label) > maxRichOutputChartText {
		return fmt.Errorf("CSV row %d column %q exceeds 120 characters", rowNumber, source.XColumn)
	}
	resolved.Labels = append(resolved.Labels, label)
	for index, columnIndex := range seriesIndexes {
		value, err := parseRichOutputCSVNumber(record[columnIndex])
		if err != nil {
			return fmt.Errorf(
				"CSV row %d column %q must be a finite number or empty",
				rowNumber,
				source.Series[index].Column,
			)
		}
		resolved.Series[index].Values = append(resolved.Series[index].Values, value)
	}
	return nil
}

func parseRichOutputCSVNumber(raw string) (*float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, fmt.Errorf("not finite")
	}
	return &number, nil
}
