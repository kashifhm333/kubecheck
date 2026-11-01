package parser

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

func TestInferScalarTypeFallback(t *testing.T) {
	cases := []struct {
		tag      string
		val      string
		expected nir.ScalarType
	}{
		{"!!int", "123", nir.TypeInt},
		{"!!float", "1.23", nir.TypeFloat},
		{"!!bool", "true", nir.TypeBool},
		{"!!null", "null", nir.TypeNull},
		{"!!str", "abc", nir.TypeString},
		// Untagged auto-inference
		{"", "null", nir.TypeNull},
		{"", "~", nir.TypeNull},
		{"", "", nir.TypeNull},
		{"", "true", nir.TypeBool},
		{"", "false", nir.TypeBool},
		{"", "456", nir.TypeInt},
		{"", "3.14", nir.TypeFloat},
		{"", "random_string", nir.TypeString},
	}

	for _, c := range cases {
		node := &yaml.Node{Tag: c.tag, Value: c.val}
		got := inferScalarType(node)
		if got != c.expected {
			t.Errorf("inferScalarType(tag=%q, val=%q) = %v, expected %v", c.tag, c.val, got, c.expected)
		}
	}
}

func TestGovernorDirectLimits(t *testing.T) {
	gov := NewGovernor(Limits{
		MaxScalarBytes:       5,
		MaxCollectionEntries: 2,
		MaxDocuments:         1,
	})

	if err := gov.CheckScalarSize(10); err == nil {
		t.Error("expected error for scalar size exceeding limit")
	}
	if err := gov.CheckScalarSize(3); err != nil {
		t.Errorf("unexpected error for valid scalar size: %v", err)
	}

	if err := gov.RecordCollectionEntry(); err != nil {
		t.Errorf("unexpected error for first collection entry: %v", err)
	}
	if err := gov.RecordCollectionEntry(); err != nil {
		t.Errorf("unexpected error for second collection entry: %v", err)
	}
	if err := gov.RecordCollectionEntry(); err == nil {
		t.Error("expected error for collection entries exceeding limit")
	}

	if err := gov.RecordDocument(); err != nil {
		t.Errorf("unexpected error for first document: %v", err)
	}
	if err := gov.RecordDocument(); err == nil {
		t.Error("expected error for document count exceeding limit")
	}
}
