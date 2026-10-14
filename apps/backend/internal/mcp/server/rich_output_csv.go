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

type richOutputResolvedSeries struct {
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

type richOutputResolvedChart struct {
	ElementID string                     `json:"element_id"`
	Labels    []string                   `json:"labels"`
	Series    []richOutputResolvedSeries `json:"series"`
}

type richOutputCSVSnapshot struct {
	Version        int                       `json:"version"`
	CatalogVersion int                       `json:"catalog_version"`
	ResolvedCharts []richOutputResolvedChart `json:"resolved_charts,omitempty"`
	ResolvedTables []richOutputResolvedTable `json:"resolved_tables,omitempty"`
}

func (s *Server) resolveRichOutputCSV(
	ctx context.Context,
	args map[string]interface{},
) (*richOutputCSVSnapshot, error) {
	presentation, err := decodeRichOutputComposition(args)
	if err != nil {
		return nil, err
	}
	snapshot := &richOutputCSVSnapshot{Version: 2, CatalogVersion: 1}
	cache := make(map[string]*streams.FileContentResponse)
	for _, id := range richOutputCSVElementIDs(presentation) {
		if presentation.Spec.Elements[id].Type == richOutputComponentTable {
			resolved, resolveErr := s.resolveRichOutputTable(ctx, presentation.Spec.Elements[id], id, cache)
			if resolveErr != nil {
				return nil, fmt.Errorf("element %s: %w", id, resolveErr)
			}
			if resolved != nil {
				snapshot.ResolvedTables = append(snapshot.ResolvedTables, *resolved)
			}
			continue
		}
		var chart richOutputChartProperties
		if err := json.Unmarshal(presentation.Spec.Elements[id].Props, &chart); err != nil {
			return nil, err
		}
		if chart.CSV == nil {
			continue
		}
		response, readErr := s.readRichOutputCSV(ctx, chart.CSV.Path, chart.CSV.Repo, cache)
		if readErr != nil {
			return nil, fmt.Errorf("element %s: %w", id, readErr)
		}
		resolved, parseErr := parseRichOutputCSV(*chart.CSV, response.Content, id)
		if parseErr != nil {
			return nil, fmt.Errorf("element %s, CSV source %s: %w", id, chart.CSV.Path, parseErr)
		}
		snapshot.ResolvedCharts = append(snapshot.ResolvedCharts, resolved)
	}
	if len(snapshot.ResolvedCharts) == 0 && len(snapshot.ResolvedTables) == 0 {
		return nil, nil
	}
	encoded, err := encodeRichOutputPayload(snapshot)
	if err != nil || len(encoded) > maxRichOutputPayloadBytes {
		return nil, fmt.Errorf("normalized CSV snapshot exceeds 64 KiB")
	}
	return snapshot, nil
}

func decodeRichOutputComposition(args map[string]interface{}) (richOutputComposition, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return richOutputComposition{}, fmt.Errorf("rich output payload is invalid")
	}
	var input richOutputComposition
	if err := json.Unmarshal(encoded, &input); err != nil {
		return richOutputComposition{}, fmt.Errorf("rich output payload is invalid")
	}
	return input, nil
}
func richOutputCSVElementIDs(input richOutputComposition) []string {
	var ids []string
	var visit func(string)
	visit = func(id string) {
		element := input.Spec.Elements[id]
		if element.Type == richOutputComponentChart || element.Type == richOutputComponentTable {
			ids = append(ids, id)
		}
		for _, child := range element.Children {
			visit(child)
		}
	}
	visit(input.Spec.Root)
	return ids
}

func (s *Server) readRichOutputCSV(
	ctx context.Context,
	filePath, repo string,
	cache map[string]*streams.FileContentResponse,
) (*streams.FileContentResponse, error) {
	cacheKey := repo + "\x00" + filePath
	if cached := cache[cacheKey]; cached != nil {
		return cached, nil
	}
	payload := map[string]interface{}{richOutputFieldSessionID: s.sessionID, richOutputFieldPath: filePath}
	if repo != "" {
		payload["repo"] = repo
	}
	var response streams.FileContentResponse
	if err := s.backend.RequestPayload(ctx, ws.ActionWorkspaceFileContentGet, payload, &response); err != nil {
		return nil, fmt.Errorf("could not read CSV source %s: %w", filePath, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("could not read CSV source %s: %s", filePath, response.Error)
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

func newRichOutputCSVReader(content string) *csv.Reader {
	return csv.NewReader(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
}

func parseRichOutputCSV(
	source richOutputCSVSourceInput,
	content string,
	elementID string,
) (richOutputResolvedChart, error) {
	reader := newRichOutputCSVReader(content)
	headerIndexes, err := readRichOutputCSVHeader(reader)
	if err != nil {
		return richOutputResolvedChart{}, err
	}
	xIndex, seriesIndexes, err := resolveRichOutputCSVColumns(source, headerIndexes)
	if err != nil {
		return richOutputResolvedChart{}, err
	}
	resolved := newRichOutputResolvedChart(source, elementID)
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
	elementID string,
) richOutputResolvedChart {
	resolved := richOutputResolvedChart{ElementID: elementID}
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
