package reporter

import (
	"encoding/json"
	"io"

	"github.com/pschrimp/virtuoso/internal/report"
)

// JSONReporter outputs reports in JSON format
type JSONReporter struct {
	writer io.Writer
}

// NewJSONReporter creates a new JSON reporter that writes to the given writer
func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{writer: w}
}

// Write outputs the report as formatted JSON
func (r *JSONReporter) Write(rep *report.Report) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(rep)
}
