package report

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// JUnitTestSuites represents the root <testsuites> element in JUnit XML.
type JUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a <testsuite> grouping findings by file.
type JUnitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents an individual check or finding.
type JUnitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
}

// JUnitFailure describes a failed validation test.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// RenderJUnit formats findings into standard JUnit XML for CI test runner reporting.
func RenderJUnit(w io.Writer, summary Summary, findings []diagnostic.Finding) error {
	durationSec := fmt.Sprintf("%.3f", float64(summary.Duration.Microseconds())/1000000.0)

	// Group findings by file
	grouped := make(map[string][]diagnostic.Finding)
	for _, f := range findings {
		file := f.Location.FilePath
		if file == "" {
			file = "stdin"
		}
		grouped[file] = append(grouped[file], f)
	}

	var fileKeys []string
	for k := range grouped {
		fileKeys = append(fileKeys, k)
	}
	sort.Strings(fileKeys)

	suites := make([]JUnitTestSuite, 0, len(fileKeys))
	totalFailures := 0
	totalTests := 0

	for _, file := range fileKeys {
		fileFindings := grouped[file]
		suiteFailures := 0
		var cases []JUnitTestCase

		for _, f := range fileFindings {
			isFailure := f.Severity == diagnostic.SeverityError
			if isFailure {
				suiteFailures++
				totalFailures++
			}
			totalTests++

			className := fmt.Sprintf("%s:%d:%d", file, f.Location.Line, f.Location.Column)
			testName := fmt.Sprintf("[%s] %s", f.RuleID, f.Message)

			tc := JUnitTestCase{
				Name:      testName,
				ClassName: className,
				Time:      "0.001",
			}

			if isFailure {
				tc.Failure = &JUnitFailure{
					Message: f.Message,
					Type:    string(f.Severity),
					Content: fmt.Sprintf("[%s] at %s\nMessage: %s\nRemediation: %s", f.RuleID, className, f.Message, f.Remediation),
				}
			}

			cases = append(cases, tc)
		}

		suites = append(suites, JUnitTestSuite{
			Name:      file,
			Tests:     len(cases),
			Failures:  suiteFailures,
			Errors:    0,
			Time:      durationSec,
			TestCases: cases,
		})
	}

	// If no findings, output a clean passing testcase
	if len(suites) == 0 {
		totalTests = 1
		suites = append(suites, JUnitTestSuite{
			Name:     "KubeCheck",
			Tests:    1,
			Failures: 0,
			Errors:   0,
			Time:     durationSec,
			TestCases: []JUnitTestCase{
				{
					Name:      "Kubernetes Manifest Diagnostics",
					ClassName: "kubecheck.analyze",
					Time:      durationSec,
				},
			},
		})
	}

	root := JUnitTestSuites{
		Name:     "KubeCheck",
		Tests:    totalTests,
		Failures: totalFailures,
		Errors:   0,
		Time:     durationSec,
		Suites:   suites,
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(root); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
