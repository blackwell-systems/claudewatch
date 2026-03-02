package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModelPricing holds per-million-token pricing used for cost estimation.
// Field names match analyzer.ModelPricing for straightforward conversion.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// TurnAttribution holds aggregated token and cost data for one tool type.
type TurnAttribution struct {
	ToolType     string  `json:"tool_type"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	EstCostUSD   float64 `json:"est_cost_usd"`
}

// attributionEntry is a minimal struct for parsing JSONL lines.
type attributionEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// attributionAssistantMsg parses the assistant message content and usage.
type attributionAssistantMsg struct {
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Content []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"content"`
}

// findTranscriptFile locates the JSONL transcript file for the given sessionID.
// If sessionID is empty, returns the most recently modified .jsonl file under
// claudeHome/projects/.
func findTranscriptFile(sessionID, claudeHome string) (string, error) {
	projectsDir := filepath.Join(claudeHome, "projects")

	if sessionID != "" {
		// Walk to find the file matching sessionID + ".jsonl".
		target := sessionID + ".jsonl"
		var found string
		err := filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr
			}
			if !d.IsDir() && d.Name() == target {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		if found == "" {
			return "", os.ErrNotExist
		}
		return found, nil
	}

	// sessionID is empty — find the most recently modified .jsonl file.
	var newestPath string
	var newestTime int64

	err := filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr
		}
		if info.ModTime().UnixNano() > newestTime {
			newestTime = info.ModTime().UnixNano()
			newestPath = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if newestPath == "" {
		return "", os.ErrNotExist
	}
	return newestPath, nil
}

// ComputeAttribution reads the JSONL transcript for sessionID under claudeHome/projects/
// and groups token usage by tool type across all assistant turns.
// Returns rows sorted by EstCostUSD descending.
// If sessionID is empty, uses the most recently modified .jsonl file.
func ComputeAttribution(sessionID, claudeHome string, pricing ModelPricing) ([]TurnAttribution, error) {
	filePath, err := findTranscriptFile(sessionID, claudeHome)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Accumulate per-tool-type stats.
	type toolStats struct {
		calls        int
		inputTokens  int
		outputTokens int
	}
	byTool := make(map[string]*toolStats)

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 10*1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry attributionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}

		var msg attributionAssistantMsg
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		inputTokens := msg.Usage.InputTokens
		outputTokens := msg.Usage.OutputTokens

		// Collect tool_use blocks.
		var toolUseNames []string
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				toolUseNames = append(toolUseNames, block.Name)
			}
		}

		// Determine the attribution key.
		var key string
		if len(toolUseNames) == 0 {
			key = "text"
		} else {
			key = toolUseNames[0]
		}

		if _, ok := byTool[key]; !ok {
			byTool[key] = &toolStats{}
		}

		// Increment call count for each tool_use block.
		if len(toolUseNames) == 0 {
			byTool[key].calls++
		} else {
			for _, name := range toolUseNames {
				if _, ok := byTool[name]; !ok {
					byTool[name] = &toolStats{}
				}
				byTool[name].calls++
			}
		}

		// Attribute tokens to the primary key (first tool_use or "text").
		byTool[key].inputTokens += inputTokens
		byTool[key].outputTokens += outputTokens
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(byTool) == 0 {
		return []TurnAttribution{}, nil
	}

	// Build result slice.
	result := make([]TurnAttribution, 0, len(byTool))
	for toolType, stats := range byTool {
		cost := (float64(stats.inputTokens)/1_000_000.0)*pricing.InputPerMillion +
			(float64(stats.outputTokens)/1_000_000.0)*pricing.OutputPerMillion
		result = append(result, TurnAttribution{
			ToolType:     toolType,
			Calls:        stats.calls,
			InputTokens:  stats.inputTokens,
			OutputTokens: stats.outputTokens,
			EstCostUSD:   cost,
		})
	}

	// Sort by EstCostUSD descending.
	sort.Slice(result, func(i, j int) bool {
		return result[i].EstCostUSD > result[j].EstCostUSD
	})

	return result, nil
}
