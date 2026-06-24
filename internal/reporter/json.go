package reporter

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Peto-RH/virtuoso/internal/destination/candlepin"
)

type JSONReporter struct {
	writer io.Writer
}

func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{writer: w}
}

func (r *JSONReporter) Write(hyp *candlepin.Hypervisor) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(hyp); err != nil {
		return fmt.Errorf("failed to encode hypervisor data: %w", err)
	}
	return nil
}
