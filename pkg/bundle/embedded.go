package bundle

import (
	"fmt"
	"os"
)

const (
	// DefaultEmbeddedBundleID is the authoritative identifier of the embedded knowledge bundle.
	DefaultEmbeddedBundleID = "kubecheck-standard-2026.10.1"

	// DefaultEmbeddedBundleVersion is the integer sequence of the embedded bundle.
	DefaultEmbeddedBundleVersion = 20261001
)

// DefaultEmbeddedManifest returns the self-contained, pre-signed fallback bundle manifest.
func DefaultEmbeddedManifest() *BundleManifest {
	m := &BundleManifest{
		FormatVersion: CurrentFormatVersion,
		BundleVersion: DefaultEmbeddedBundleVersion,
		BundleID:      DefaultEmbeddedBundleID,
		EngineCompatibility: EngineCompatibility{
			MinVersion: "1.0.0",
			MaxVersion: "1.99.99",
		},
		KubernetesVersions: []KubernetesVersion{
			{Minor: "1.24", PatchPin: "1.24.17", SchemaVersion: "1.0.0", Status: "deprecated", SupportedUntil: "2027-01-01"},
			{Minor: "1.25", PatchPin: "1.25.16", SchemaVersion: "1.0.0", Status: "deprecated", SupportedUntil: "2027-01-01"},
			{Minor: "1.26", PatchPin: "1.26.15", SchemaVersion: "1.0.0", Status: "deprecated", SupportedUntil: "2027-01-01"},
			{Minor: "1.27", PatchPin: "1.27.16", SchemaVersion: "1.0.0", Status: "supported"},
			{Minor: "1.28", PatchPin: "1.28.15", SchemaVersion: "1.0.0", Status: "supported"},
			{Minor: "1.29", PatchPin: "1.29.10", SchemaVersion: "1.0.0", Status: "supported"},
			{Minor: "1.30", PatchPin: "1.30.6", SchemaVersion: "1.0.0", Status: "supported"},
			{Minor: "1.31", PatchPin: "1.31.2", SchemaVersion: "1.0.0", Status: "supported"},
		},
		Generation: GenerationMetadata{
			Timestamp:        "2026-10-31T00:00:00Z",
			GeneratorVersion: "1.0.0-dev",
			SourceCommit:     "a0423d387ffba4bc099f69742c3d0b2e8a1d5291",
			Reproducible:     true,
			FreshUntil:       "2027-04-30T00:00:00Z",
		},
		Provenance: []ProvenanceSource{
			{
				SourceID:      "kubernetes-upstream",
				SourceType:    "kubernetes-source",
				SourceURL:     "https://github.com/kubernetes/kubernetes",
				SourceVersion: "v1.31.2",
				RetrievedAt:   "2026-10-31T00:00:00Z",
			},
		},
		Files: []BundleFile{
			{
				Path:   "schemas/index.json",
				SHA256: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Size:   0,
				Kind:   "index",
			},
		},
		Limits: BundleLimits{
			MaxTotalSize:        524288000,
			MaxFiles:            100000,
			MaxSchemaSize:       16777216,
			MaxRulePackSize:     33554432,
			MaxIndexEntries:     500000,
			MaxDecompressedSize: 2147483648,
		},
		Licenses: []string{"Apache-2.0"},
	}

	// Sign using the default deterministic release key
	_, priv := DefaultKeyPair()
	_ = SignManifest(m, priv, DefaultReleaseKeyID)

	return m
}

// Resolver resolves the active knowledge bundle following the strict precedence:
// 1. Explicit CLI --bundle path
// 2. KUBECHECK_BUNDLE environment variable
// 3. Local bundle store active bundle ($XDG_DATA_HOME/kubecheck/bundles/bundle)
// 4. Default embedded bundle fallback
type Resolver struct {
	store     *Store
	validator *Validator
}

// NewResolver initializes a bundle Resolver.
func NewResolver(store *Store, validator *Validator) *Resolver {
	return &Resolver{
		store:     store,
		validator: validator,
	}
}

// Resolve identifies and loads the active knowledge bundle.
func (r *Resolver) Resolve(explicitPath string) (*BundleManifest, string, error) {
	// 1. Explicit path
	if explicitPath != "" {
		m, _, err := r.validator.ValidateDir(explicitPath, 0)
		if err != nil {
			return nil, "", fmt.Errorf("explicit bundle at %q failed validation: %w", explicitPath, err)
		}
		return m, fmt.Sprintf("Explicit: %s", explicitPath), nil
	}

	// 2. Environment variable
	if envPath := os.Getenv("KUBECHECK_BUNDLE"); envPath != "" {
		m, _, err := r.validator.ValidateDir(envPath, 0)
		if err != nil {
			return nil, "", fmt.Errorf("environment bundle at %q failed validation: %w", envPath, err)
		}
		return m, fmt.Sprintf("Environment: %s", envPath), nil
	}

	// 3. Store active bundle
	if r.store != nil {
		if m, err := r.store.GetActiveManifest(); err == nil && m != nil {
			// Re-verify the store active bundle
			if _, _, vErr := r.validator.ValidateDir(r.store.ActiveBundleDir(), 0); vErr == nil {
				return m, fmt.Sprintf("Installed Store: %s", r.store.ActiveBundleDir()), nil
			}
		}
	}

	// 4. Embedded fallback
	embedded := DefaultEmbeddedManifest()
	return embedded, "Embedded Default", nil
}
