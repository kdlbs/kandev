package mcp

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type richOutputTableColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}
type richOutputTableFileSource struct {
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
}
type richOutputTableProperties struct {
	Columns []richOutputTableColumn    `json:"columns"`
	Rows    [][]string                 `json:"rows"`
	CSV     *richOutputTableFileSource `json:"csv,omitempty"`
}
type richOutputResolvedTable struct {
	ElementID string                  `json:"element_id"`
	Columns   []richOutputTableColumn `json:"columns"`
	Rows      [][]string              `json:"rows"`
}

func validateRichOutputTable(element richOutputElement) error {
	var table richOutputTableProperties
	if err := json.Unmarshal(element.Props, &table); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, column := range table.Columns {
		if seen[column.Key] {
			return fmt.Errorf("table column keys must be unique")
		}
		seen[column.Key] = true
	}
	if table.CSV != nil {
		if !isWorkspaceRelativeRichOutputPath(table.CSV.Path) {
			return fmt.Errorf("CSV path must be workspace relative")
		}
		if !strings.EqualFold(path.Ext(table.CSV.Path), ".csv") {
			return fmt.Errorf("CSV path must end in .csv")
		}
		return nil
	}
	for _, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return fmt.Errorf("table row width must match columns")
		}
	}
	return nil
}

func (s *Server) resolveRichOutputTable(ctx context.Context, element richOutputElement, id string, cache map[string]*streams.FileContentResponse) (*richOutputResolvedTable, error) {
	var table richOutputTableProperties
	if err := json.Unmarshal(element.Props, &table); err != nil {
		return nil, err
	}
	if table.CSV == nil {
		return nil, nil
	}
	response, err := s.readRichOutputCSV(ctx, table.CSV.Path, table.CSV.Repo, cache)
	if err != nil {
		return nil, err
	}
	return parseRichOutputTableCSV(table, response.Content, id)
}

func readRichOutputCSVHeader(reader *csv.Reader) (map[string]int, error) {
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("CSV source must contain a header row")
	}
	if err != nil {
		return nil, fmt.Errorf("CSV header is invalid: %w", err)
	}
	indexes, err := indexRichOutputCSVHeaders(header)
	if err != nil {
		return nil, err
	}
	reader.FieldsPerRecord = len(header)
	return indexes, nil
}

func parseRichOutputTableCSV(table richOutputTableProperties, content, id string) (*richOutputResolvedTable, error) {
	reader := newRichOutputCSVReader(content)
	headers, err := readRichOutputCSVHeader(reader)
	if err != nil {
		return nil, err
	}
	indexes := make([]int, len(table.Columns))
	for i, column := range table.Columns {
		index, ok := headers[column.Key]
		if !ok {
			return nil, fmt.Errorf("CSV column %q was not found", column.Key)
		}
		indexes[i] = index
	}
	result := &richOutputResolvedTable{ElementID: id, Columns: table.Columns, Rows: make([][]string, 0)}
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("CSV row %d is invalid: %w", rowNumber, readErr)
		}
		if len(result.Rows) == maxRichOutputCSVRows {
			return nil, fmt.Errorf("CSV source must contain at most 100 data rows")
		}
		row := make([]string, len(indexes))
		for i, index := range indexes {
			if utf8.RuneCountInString(record[index]) > 500 {
				return nil, fmt.Errorf("CSV row %d column %q exceeds 500 characters", rowNumber, table.Columns[i].Key)
			}
			row[i] = record[index]
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}
