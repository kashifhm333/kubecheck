package report

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// SARIFReport represents the root object of an OASIS SARIF v2.1.0 document.
type SARIFReport struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun describes an execution of the KubeCheck diagnostic engine.
type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

// SARIFTool describes the analyzer driver and rule catalog.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver encapsulates tool metadata and registered rules.
type SARIFDriver struct {
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	InformationURI string          `json:"informationUri"`
	Rules          []SARIFRuleMeta `json:"rules"`
}

// SARIFRuleMeta specifies rule metadata in the driver catalog.
type SARIFRuleMeta struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	ShortDescription     SARIFMessage           `json:"shortDescription"`
	DefaultConfiguration SARIFRuleConfiguration `json:"defaultConfiguration"`
}

// SARIFRuleConfiguration defines default reporting levels for rules.
type SARIFRuleConfiguration struct {
	Level string `json:"level"`
}

// SARIFResult represents an individual diagnostic alert in SARIF.
type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"` // "error", "warning", "note"
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations,omitempty"`
}

// SARIFMessage wraps human-readable text.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFLocation anchors a finding to a source artifact and coordinate region.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation specifies the file URI and text region.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

// SARIFArtifactLocation holds the file URI.
type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

// SARIFRegion contains 1-based text coordinates.
type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// RenderSARIF serializes findings to OASIS SARIF v2.1.0 JSON format.
func RenderSARIF(w io.Writer, summary Summary, findings []diagnostic.Finding) error {
	rulesMap := make(map[string]SARIFRuleMeta)
	results := make([]SARIFResult, 0, len(findings))

	for _, f := range findings {
		sarifLevel := toSARIFLevel(f.Severity)

		if _, exists := rulesMap[f.RuleID]; !exists {
			rulesMap[f.RuleID] = SARIFRuleMeta{
				ID:   f.RuleID,
				Name: f.RuleID,
				ShortDescription: SARIFMessage{
					Text: f.RuleID,
				},
				DefaultConfiguration: SARIFRuleConfiguration{
					Level: sarifLevel,
				},
			}
		}

		line := f.Location.Line
		if line < 1 {
			line = 1
		}
		col := f.Location.Column
		if col < 1 {
			col = 1
		}

		uri := f.Location.FilePath
		if uri == "" || uri == "-" {
			uri = "stdin"
		}

		res := SARIFResult{
			RuleID: f.RuleID,
			Level:  sarifLevel,
			Message: SARIFMessage{
				Text: f.Message,
			},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: uri,
						},
						Region: SARIFRegion{
							StartLine:   line,
							StartColumn: col,
							EndLine:     line,
							EndColumn:   col,
						},
					},
				},
			},
		}

		results = append(results, res)
	}

	var rules []SARIFRuleMeta
	for _, r := range rulesMap {
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})

	engineVer := summary.EngineVersion
	if engineVer == "" {
		engineVer = "1.0.0-dev"
	}

	report := SARIFReport{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:           "KubeCheck",
						Version:        engineVer,
						InformationURI: "https://github.com/kubecheck/kubecheck",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

func toSARIFLevel(sev diagnostic.Severity) string {
	switch strings.ToLower(string(sev)) {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}
