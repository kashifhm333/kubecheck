package nir_test

import (
	"testing"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

func TestNodeGetPath(t *testing.T) {
	// Construct a synthetic NIR mapping:
	// metadata:
	//   name: my-deployment
	// spec:
	//   replicas: 3
	//   paused: true

	nameNode := &nir.Node{
		Kind:       nir.KindScalar,
		Value:      "my-deployment",
		ScalarType: nir.TypeString,
		Location:   nir.Location{FilePath: "test.yaml", Line: 3, Column: 9},
	}
	metaNode := &nir.Node{
		Kind:     nir.KindMapping,
		Location: nir.Location{FilePath: "test.yaml", Line: 2, Column: 1},
		Fields: []nir.MappingEntry{
			{Key: &nir.Node{Kind: nir.KindScalar, Value: "name"}, Value: nameNode},
		},
	}

	replicasNode := &nir.Node{
		Kind:       nir.KindScalar,
		Value:      "3",
		ScalarType: nir.TypeInt,
		Location:   nir.Location{FilePath: "test.yaml", Line: 5, Column: 13},
	}
	pausedNode := &nir.Node{
		Kind:       nir.KindScalar,
		Value:      "true",
		ScalarType: nir.TypeBool,
		Location:   nir.Location{FilePath: "test.yaml", Line: 6, Column: 11},
	}
	specNode := &nir.Node{
		Kind:     nir.KindMapping,
		Location: nir.Location{FilePath: "test.yaml", Line: 4, Column: 1},
		Fields: []nir.MappingEntry{
			{Key: &nir.Node{Kind: nir.KindScalar, Value: "replicas"}, Value: replicasNode},
			{Key: &nir.Node{Kind: nir.KindScalar, Value: "paused"}, Value: pausedNode},
		},
	}

	root := &nir.Node{
		Kind:     nir.KindMapping,
		Location: nir.Location{FilePath: "test.yaml", Line: 1, Column: 1},
		Fields: []nir.MappingEntry{
			{Key: &nir.Node{Kind: nir.KindScalar, Value: "metadata"}, Value: metaNode},
			{Key: &nir.Node{Kind: nir.KindScalar, Value: "spec"}, Value: specNode},
		},
	}

	// Test Get
	foundName := root.Get("metadata.name")
	if foundName == nil || foundName.Value != "my-deployment" {
		t.Errorf("expected my-deployment, got %v", foundName)
	}

	// Test GetString
	val, ok := root.GetString("metadata.name")
	if !ok || val != "my-deployment" {
		t.Errorf("GetString failed, got %s (ok=%t)", val, ok)
	}

	// Test GetInt
	rep, ok := root.GetInt("spec.replicas")
	if !ok || rep != 3 {
		t.Errorf("GetInt failed, got %d (ok=%t)", rep, ok)
	}

	// Test GetBool
	paused, ok := root.GetBool("spec.paused")
	if !ok || !paused {
		t.Errorf("GetBool failed, got %t (ok=%t)", paused, ok)
	}

	// Test Nonexistent Path
	if missing := root.Get("spec.nonexistent.field"); missing != nil {
		t.Errorf("expected nil for missing path, got %v", missing)
	}

	// Test ToMap
	rawMap, ok := root.ToMap().(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", root.ToMap())
	}
	specMap, ok := rawMap["spec"].(map[string]any)
	if !ok || specMap["replicas"] != int64(3) || specMap["paused"] != true {
		t.Errorf("unexpected ToMap conversion: %+v", rawMap)
	}
}

func TestNIRNodeTypesAndStrings(t *testing.T) {
	loc := nir.Location{FilePath: "test.yaml", Line: 10, Column: 4, DocumentIndex: 0}
	if loc.String() != "test.yaml:10:4 (doc #0)" {
		t.Errorf("unexpected Location.String(): %s", loc.String())
	}

	// Kinds and Scalar Types
	if nir.KindScalar.String() != "scalar" || nir.KindMapping.String() != "mapping" ||
		nir.KindSequence.String() != "sequence" || nir.KindNull.String() != "null" {
		t.Errorf("unexpected Kind.String() values")
	}

	scalarTypes := []nir.ScalarType{nir.TypeString, nir.TypeInt, nir.TypeFloat, nir.TypeBool, nir.TypeNull, nir.TypeUnknown}
	for _, st := range scalarTypes {
		if st.String() == "" {
			t.Errorf("empty string for scalar type %v", st)
		}
	}

	// Sequence node and ToMap
	item1 := &nir.Node{Kind: nir.KindScalar, Value: "item1", ScalarType: nir.TypeString}
	item2 := &nir.Node{Kind: nir.KindScalar, Value: "12.34", ScalarType: nir.TypeFloat}
	nullItem := &nir.Node{Kind: nir.KindNull}

	seqNode := &nir.Node{
		Kind:  nir.KindSequence,
		Items: []*nir.Node{item1, item2, nullItem},
	}

	slice, ok := seqNode.ToMap().([]any)
	if !ok || len(slice) != 3 {
		t.Fatalf("expected []any with 3 elements, got %T (%v)", seqNode.ToMap(), seqNode.ToMap())
	}
	if slice[0] != "item1" || slice[1] != 12.34 || slice[2] != nil {
		t.Errorf("unexpected slice values: %+v", slice)
	}

	// Document structure
	doc := &nir.Document{
		Index:    0,
		Location: nir.Location{FilePath: "test.yaml"},
		Root:     seqNode,
	}
	if doc.Root.Kind != nir.KindSequence {
		t.Errorf("expected sequence root")
	}
}

func TestNIRAccessorEdgeCases(t *testing.T) {
	var nilNode *nir.Node
	if nilNode.ToMap() != nil {
		t.Error("expected ToMap on nil node to return nil")
	}

	root := &nir.Node{
		Kind: nir.KindMapping,
		Fields: []nir.MappingEntry{
			{
				Key:   &nir.Node{Kind: nir.KindScalar, Value: "not_an_int"},
				Value: &nir.Node{Kind: nir.KindScalar, Value: "not_a_number"},
			},
			{
				Key:   &nir.Node{Kind: nir.KindScalar, Value: "not_a_bool"},
				Value: &nir.Node{Kind: nir.KindScalar, Value: "maybe"},
			},
			{
				Key:   &nir.Node{Kind: nir.KindScalar, Value: "mapping_node"},
				Value: &nir.Node{Kind: nir.KindMapping},
			},
		},
	}

	if _, ok := root.GetString("nonexistent"); ok {
		t.Error("expected GetString on missing field to return ok=false")
	}
	if _, ok := root.GetString("mapping_node"); ok {
		t.Error("expected GetString on mapping node to return ok=false")
	}

	if _, ok := root.GetInt("nonexistent"); ok {
		t.Error("expected GetInt on missing field to return ok=false")
	}
	if _, ok := root.GetInt("mapping_node"); ok {
		t.Error("expected GetInt on mapping node to return ok=false")
	}
	if _, ok := root.GetInt("not_an_int"); ok {
		t.Error("expected GetInt on invalid int value to return ok=false")
	}

	if _, ok := root.GetBool("nonexistent"); ok {
		t.Error("expected GetBool on missing field to return ok=false")
	}
	if _, ok := root.GetBool("mapping_node"); ok {
		t.Error("expected GetBool on mapping node to return ok=false")
	}
	if _, ok := root.GetBool("not_a_bool"); ok {
		t.Error("expected GetBool on invalid bool value to return ok=false")
	}

	// Test Get with nil receiver and empty segments
	if nilNode.Get("any.path") != nil {
		t.Error("expected Get on nil node to return nil")
	}
	if root.Get("") != root {
		t.Error("expected Get('') to return root node")
	}
}
