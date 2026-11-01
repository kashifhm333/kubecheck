package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// CurrentFormatVersion is the normative bundle format version per KNOWLEDGE_BUNDLE_FORMAT.md.
const CurrentFormatVersion = "1.0.0"

// BundleManifest describes the complete metadata, schema versions, files, and signatures of a knowledge bundle.
type BundleManifest struct {
	FormatVersion       string              `json:"format_version"`
	BundleVersion       int                 `json:"bundle_version"`
	BundleID            string              `json:"bundle_id"`
	EngineCompatibility EngineCompatibility `json:"engine_compatibility"`
	KubernetesVersions  []KubernetesVersion `json:"kubernetes_versions"`
	Generation          GenerationMetadata  `json:"generation"`
	Provenance          []ProvenanceSource  `json:"provenance"`
	Files               []BundleFile        `json:"files"`
	Partitions          []Partition         `json:"partitions,omitempty"`
	Limits              BundleLimits        `json:"limits"`
	Licenses            []string            `json:"licenses"`
	Signature           SignatureBlock      `json:"signature"`
}

// EngineCompatibility defines the minimum and maximum supported engine versions.
type EngineCompatibility struct {
	MinVersion string `json:"min_version"`
	MaxVersion string `json:"max_version"`
}

// KubernetesVersion documents the schema and lifecycle status of a specific Kubernetes release.
type KubernetesVersion struct {
	Minor          string `json:"minor"`
	PatchPin       string `json:"patch_pin"`
	SchemaVersion  string `json:"schema_version"`
	Status         string `json:"status"` // "supported" or "deprecated"
	SupportedUntil string `json:"supported_until,omitempty"`
}

// GenerationMetadata captures provenance details about how the bundle was built.
type GenerationMetadata struct {
	Timestamp        string `json:"timestamp"`
	GeneratorVersion string `json:"generator_version"`
	SourceCommit     string `json:"source_commit"`
	Reproducible     bool   `json:"reproducible"`
	FreshUntil       string `json:"fresh_until,omitempty"`
}

// ProvenanceSource cites the authoritative origin of knowledge in the bundle.
type ProvenanceSource struct {
	SourceID      string `json:"source_id"`
	SourceType    string `json:"source_type"`
	SourceURL     string `json:"source_url,omitempty"`
	SourcePath    string `json:"source_path,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	RetrievedAt   string `json:"retrieved_at"`
	ContentHash   string `json:"content_hash,omitempty"`
}

// BundleFile describes an individual content file contained in the bundle.
type BundleFile struct {
	Path             string `json:"path"`
	SHA256           string `json:"sha256"`
	Size             int64  `json:"size"`
	Kind             string `json:"kind"`
	Compression      string `json:"compression,omitempty"`
	DecompressedSize int64  `json:"decompressed_size,omitempty"`
}

// Partition groups bundle files into logical subsets.
type Partition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Files       []string `json:"files"`
}

// BundleLimits contains integrity bounds verified by the loader.
type BundleLimits struct {
	MaxTotalSize        int64 `json:"max_total_size"`
	MaxFiles            int   `json:"max_files"`
	MaxSchemaSize       int64 `json:"max_schema_size"`
	MaxRulePackSize     int64 `json:"max_rule_pack_size"`
	MaxIndexEntries     int   `json:"max_index_entries"`
	MaxDecompressedSize int64 `json:"max_decompressed_size"`
}

// SignatureBlock contains cryptographic authentication details.
type SignatureBlock struct {
	Algorithm          string `json:"algorithm"`
	KeyID              string `json:"key_id"`
	Value              string `json:"value"` // base64-encoded signature
	SignedManifestHash string `json:"signed_manifest_hash"`
}

// CanonicalPayload returns the exact byte payload covered by the Ed25519 digital signature per UPDATE-SIGN-REQ-010:
// (1) Canonical JSON serialization of the manifest with the Signature block omitted
// (2) Concatenated with the sorted list of "(path, sha256)\n" pairs of all declared files.
func (m *BundleManifest) CanonicalPayload() ([]byte, error) {
	// Create shallow copy with empty signature
	copyManifest := *m
	copyManifest.Signature = SignatureBlock{}

	manifestBytes, err := json.Marshal(copyManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize canonical manifest: %w", err)
	}

	manifestDigest := sha256.Sum256(manifestBytes)
	manifestHashHex := "sha256:" + hex.EncodeToString(manifestDigest[:])

	// Sort file list by path to guarantee deterministic order
	type fileEntry struct {
		path   string
		sha256 string
	}
	sortedFiles := make([]fileEntry, len(m.Files))
	for i, f := range m.Files {
		sortedFiles[i] = fileEntry{path: f.Path, sha256: f.SHA256}
	}
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].path < sortedFiles[j].path
	})

	var payload []byte
	payload = append(payload, []byte("MANIFEST_HASH:"+manifestHashHex+"\n")...)
	for _, f := range sortedFiles {
		payload = append(payload, []byte(fmt.Sprintf("%s|%s\n", f.path, f.sha256))...)
	}

	return payload, nil
}
