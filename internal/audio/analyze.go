package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type AnalysisResult struct {
	BPM float64 `json:"bpm"`
	Key string  `json:"key"`
}

func AnalyzeWithPython(ctx context.Context, pythonPath, scriptPath, inputPath string, timeout time.Duration) (*AnalysisResult, error) {
	// pythonPath: e.g. "python" or full path to venv python
	// scriptPath: path to tools/analyze.py
	// inputPath: audio file
	cmd := exec.CommandContext(ctx, pythonPath, scriptPath, inputPath)
	// set timeout via context
	out, err := cmd.Output()
	if err != nil {
		// include stderr? use cmd.CombinedOutput() for more details
		combined, _ := cmd.CombinedOutput()
		return nil, fmt.Errorf("python analyze failed: %w | output: %s", err, string(combined))
	}
	var r AnalysisResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("invalid json from analyzer: %w | out: %s", err, string(out))
	}
	return &r, nil
}
