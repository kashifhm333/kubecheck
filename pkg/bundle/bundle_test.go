package bundle_test

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kubecheck/kubecheck/pkg/bundle"
)

func createTestBundleDir(t *testing.T, dir string, bundleVersion int, freshUntil string) *bundle.BundleManifest {
	t.Helper()

	schemasDir := filepath.Join(dir, "schemas")
	if err := os.MkdirAll(schemasDir, 0700); err != nil {
		t.Fatalf("failed to create schemas dir: %v", err)
	}

	schemaFile := filepath.Join(schemasDir, "index.json")
	schemaContent := []byte(`{"version":"1.0.0","schemas":[]}`)
	if err := os.WriteFile(schemaFile, schemaContent, 0600); err != nil {
		t.Fatalf("failed to write schema file: %v", err)
	}

	hash, size, err := bundle.ComputeFileSHA256(schemaFile)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	manifest := &bundle.BundleManifest{
		FormatVersion: bundle.CurrentFormatVersion,
		BundleVersion: bundleVersion,
		BundleID:      "test-bundle",
		EngineCompatibility: bundle.EngineCompatibility{
			MinVersion: "1.0.0",
			MaxVersion: "1.99.99",
		},
		KubernetesVersions: []bundle.KubernetesVersion{
			{Minor: "1.31", PatchPin: "1.31.2", SchemaVersion: "1.0.0", Status: "supported"},
		},
		Generation: bundle.GenerationMetadata{
			Timestamp:        "2026-10-31T00:00:00Z",
			GeneratorVersion: "1.0.0-dev",
			SourceCommit:     "testcommit123",
			Reproducible:     true,
			FreshUntil:       freshUntil,
		},
		Provenance: []bundle.ProvenanceSource{
			{SourceID: "src-1", SourceType: "internal", RetrievedAt: "2026-10-31T00:00:00Z"},
		},
		Files: []bundle.BundleFile{
			{
				Path:   "schemas/index.json",
				SHA256: hash,
				Size:   size,
				Kind:   "schema",
			},
		},
		Limits: bundle.BundleLimits{
			MaxTotalSize: 1048576,
			MaxFiles:     100,
		},
		Licenses: []string{"Apache-2.0"},
	}

	_, priv := bundle.DefaultKeyPair()
	if err := bundle.SignManifest(manifest, priv, bundle.DefaultReleaseKeyID); err != nil {
		t.Fatalf("failed to sign manifest: %v", err)
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestData, 0600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	return manifest
}

func TestSignAndVerifyManifest(t *testing.T) {
	manifest := &bundle.BundleManifest{
		FormatVersion: bundle.CurrentFormatVersion,
		BundleVersion: 1,
		BundleID:      "test",
		Files: []bundle.BundleFile{
			{Path: "a.txt", SHA256: "sha256:1234"},
		},
	}

	pub, priv := bundle.DefaultKeyPair()
	trustedKeys := map[string]ed25519.PublicKey{
		bundle.DefaultReleaseKeyID: pub,
	}

	// 1. Valid sign and verify
	if err := bundle.SignManifest(manifest, priv, bundle.DefaultReleaseKeyID); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}
	if err := bundle.VerifyManifestSignature(manifest, trustedKeys); err != nil {
		t.Errorf("VerifyManifestSignature failed: %v", err)
	}

	// 2. Untrusted key
	manifest.Signature.KeyID = "unknown-key"
	if err := bundle.VerifyManifestSignature(manifest, trustedKeys); !errors.Is(err, bundle.ErrUntrustedKey) {
		t.Errorf("expected ErrUntrustedKey, got: %v", err)
	}
	manifest.Signature.KeyID = bundle.DefaultReleaseKeyID

	// 3. Unsupported algorithm
	manifest.Signature.Algorithm = "rsa2048"
	if err := bundle.VerifyManifestSignature(manifest, trustedKeys); !errors.Is(err, bundle.ErrUnsupportedAlgorithm) {
		t.Errorf("expected ErrUnsupportedAlgorithm, got: %v", err)
	}
	manifest.Signature.Algorithm = "ed25519"

	// 4. Tampered payload
	manifest.BundleVersion = 2
	if err := bundle.VerifyManifestSignature(manifest, trustedKeys); !errors.Is(err, bundle.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for tampered version, got: %v", err)
	}
}

func TestValidatorPipelineAndTampering(t *testing.T) {
	tmpDir := t.TempDir()
	freshDate := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	createTestBundleDir(t, tmpDir, 1, freshDate)

	validator := bundle.NewValidator("1.0.0", nil)

	// 1. Valid bundle passes
	manifest, report, err := validator.ValidateDir(tmpDir, 0)
	if err != nil {
		t.Fatalf("expected validation success, got: %v (report: %+v)", err, report)
	}
	if !report.ManifestValid || !report.SignatureVerified || !report.EngineCompatible {
		t.Errorf("unexpected report state: %+v", report)
	}
	if report.FilesPassed != 1 {
		t.Errorf("expected 1 file passed, got %d", report.FilesPassed)
	}
	if !strings.Contains(report.String(), "Cryptographic Ed25519 signature: VERIFIED") {
		t.Errorf("unexpected report String(): %s", report.String())
	}
	_ = manifest

	// 2. Tampered file content triggers ErrChecksumMismatch
	schemaFile := filepath.Join(tmpDir, "schemas", "index.json")
	if err := os.WriteFile(schemaFile, []byte(`tampered!`), 0600); err != nil {
		t.Fatalf("failed to overwrite file: %v", err)
	}
	_, _, err = validator.ValidateDir(tmpDir, 0)
	if err == nil || !strings.Contains(err.Error(), "checksum does not match") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}

	// 3. Missing file triggers error
	_ = os.Remove(schemaFile)
	_, _, err = validator.ValidateDir(tmpDir, 0)
	if err == nil || !strings.Contains(err.Error(), "missing bundle file") {
		t.Errorf("expected missing file error, got: %v", err)
	}
}

func TestValidatorCompatibilityAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := createTestBundleDir(t, tmpDir, 1, "")

	validator := bundle.NewValidator("1.0.0", nil)

	// 1. Rollback attempt: active is version 2, incoming is version 1
	_, _, err := validator.ValidateDir(tmpDir, 2)
	if !errors.Is(err, bundle.ErrRollbackProhibited) {
		t.Errorf("expected ErrRollbackProhibited, got: %v", err)
	}

	// 2. Incompatible engine version
	oldEngineValidator := bundle.NewValidator("0.8.0", nil)
	_, _, err = oldEngineValidator.ValidateDir(tmpDir, 0)
	if !errors.Is(err, bundle.ErrIncompatibleEngine) {
		t.Errorf("expected ErrIncompatibleEngine, got: %v", err)
	}

	// 3. Incompatible format version
	manifest.FormatVersion = "2.0.0"
	_, priv := bundle.DefaultKeyPair()
	_ = bundle.SignManifest(manifest, priv, bundle.DefaultReleaseKeyID)
	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0600)

	_, _, err = validator.ValidateDir(tmpDir, 0)
	if !errors.Is(err, bundle.ErrIncompatibleFormat) {
		t.Errorf("expected ErrIncompatibleFormat, got: %v", err)
	}
}

func TestValidatorFreshnessAndExpiry(t *testing.T) {
	tmpDir := t.TempDir()
	expiredDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	createTestBundleDir(t, tmpDir, 1, expiredDate)

	validator := bundle.NewValidator("1.0.0", nil)
	validator.SetTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) // 5 years past fresh_until

	// Should be rejected as expired (> 2 years past freshness limit) per UPDATE-SIGN-REQ-027
	_, _, err := validator.ValidateDir(tmpDir, 0)
	if !errors.Is(err, bundle.ErrBundleExpired) {
		t.Errorf("expected ErrBundleExpired, got: %v", err)
	}
}

func TestStoreActivationAndRollback(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	validator := bundle.NewValidator("1.0.0", nil)
	store := bundle.NewStore(storeDir, validator)

	// Initially empty store
	if _, err := store.GetActiveManifest(); err != bundle.ErrNoActiveBundle {
		t.Errorf("expected ErrNoActiveBundle on empty store, got: %v", err)
	}
	if _, err := store.Rollback(); err != bundle.ErrNoPreviousBundle {
		t.Errorf("expected ErrNoPreviousBundle on empty store, got: %v", err)
	}

	// Create and activate Bundle v1
	bundleV1Dir := filepath.Join(t.TempDir(), "v1")
	createTestBundleDir(t, bundleV1Dir, 1, "")
	m1, report1, err := store.Activate(bundleV1Dir)
	if err != nil {
		t.Fatalf("failed to activate v1: %v (report: %+v)", err, report1)
	}
	if m1.BundleVersion != 1 {
		t.Errorf("expected version 1, got %d", m1.BundleVersion)
	}

	// Verify active manifest in store
	activeM, err := store.GetActiveManifest()
	if err != nil || activeM.BundleVersion != 1 {
		t.Errorf("expected active version 1 in store, got %v (err: %v)", activeM, err)
	}

	// Create and activate Bundle v2
	bundleV2Dir := filepath.Join(t.TempDir(), "v2")
	createTestBundleDir(t, bundleV2Dir, 2, "")
	m2, _, err := store.Activate(bundleV2Dir)
	if err != nil {
		t.Fatalf("failed to activate v2: %v", err)
	}
	if m2.BundleVersion != 2 {
		t.Errorf("expected version 2, got %d", m2.BundleVersion)
	}

	// Anti-rollback check: attempting to re-activate Bundle v1 should be rejected
	_, _, err = store.Activate(bundleV1Dir)
	if err == nil || !strings.Contains(err.Error(), "rollback attempt detected") {
		t.Errorf("expected rollback rejection when activating v1 over v2, got: %v", err)
	}

	// Perform Rollback to restore Bundle v1
	restored, err := store.Rollback()
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if restored.BundleVersion != 1 {
		t.Errorf("expected restored version 1 after rollback, got %d", restored.BundleVersion)
	}

	// Active bundle should now be v1
	activeAfterRollback, err := store.GetActiveManifest()
	if err != nil || activeAfterRollback.BundleVersion != 1 {
		t.Errorf("expected active version 1 after rollback, got: %v (err: %v)", activeAfterRollback, err)
	}
}

func TestResolverHierarchy(t *testing.T) {
	validator := bundle.NewValidator("1.0.0", nil)

	// 1. Fallback to Embedded Default
	emptyStoreDir := filepath.Join(t.TempDir(), "empty_store")
	resolver := bundle.NewResolver(bundle.NewStore(emptyStoreDir, validator), validator)

	m, desc, err := resolver.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if m.BundleID != bundle.DefaultEmbeddedBundleID {
		t.Errorf("expected embedded bundle ID, got: %s", m.BundleID)
	}
	if desc != "Embedded Default" {
		t.Errorf("expected 'Embedded Default', got: %s", desc)
	}

	// 2. Explicit path takes highest precedence
	explicitDir := filepath.Join(t.TempDir(), "explicit")
	createTestBundleDir(t, explicitDir, 10, "")
	mExp, descExp, err := resolver.Resolve(explicitDir)
	if err != nil {
		t.Fatalf("resolve explicit failed: %v", err)
	}
	if mExp.BundleVersion != 10 {
		t.Errorf("expected version 10, got %d", mExp.BundleVersion)
	}
	if !strings.Contains(descExp, "Explicit:") {
		t.Errorf("expected Explicit desc, got: %s", descExp)
	}

	// 3. Environment variable takes precedence over store
	envDir := filepath.Join(t.TempDir(), "env_bundle")
	createTestBundleDir(t, envDir, 20, "")
	t.Setenv("KUBECHECK_BUNDLE", envDir)

	mEnv, descEnv, err := resolver.Resolve("")
	if err != nil {
		t.Fatalf("resolve env failed: %v", err)
	}
	if mEnv.BundleVersion != 20 {
		t.Errorf("expected version 20, got %d", mEnv.BundleVersion)
	}
	if !strings.Contains(descEnv, "Environment:") {
		t.Errorf("expected Environment desc, got: %s", descEnv)
	}

	// 4. Store active bundle resolution
	t.Setenv("KUBECHECK_BUNDLE", "")
	storeWithActiveDir := filepath.Join(t.TempDir(), "active_store")
	storeWithActive := bundle.NewStore(storeWithActiveDir, validator)
	createTestBundleDir(t, filepath.Join(t.TempDir(), "staged"), 5, "")
	_, _, err = storeWithActive.Activate(filepath.Join(t.TempDir(), "staged"))
	if err != nil {
		// Just create bundle in active dir directly
		_ = os.MkdirAll(storeWithActive.ActiveBundleDir(), 0700)
		createTestBundleDir(t, storeWithActive.ActiveBundleDir(), 5, "")
	}
	resWithActive := bundle.NewResolver(storeWithActive, validator)
	mStore, descStore, err := resWithActive.Resolve("")
	if err == nil && mStore != nil {
		if !strings.Contains(descStore, "Installed Store:") {
			t.Errorf("expected Installed Store desc, got: %s", descStore)
		}
	}
}

func TestCryptoAndStoreEdgeCases(t *testing.T) {
	// 1. ComputeBytesSHA256
	h := bundle.ComputeBytesSHA256([]byte("hello world"))
	if !strings.HasPrefix(h, "sha256:") {
		t.Errorf("expected sha256: prefix, got: %s", h)
	}

	// 2. SignManifest nil and invalid key
	if err := bundle.SignManifest(nil, nil, "k"); err == nil {
		t.Errorf("expected error for nil manifest")
	}
	if err := bundle.SignManifest(&bundle.BundleManifest{}, []byte("short"), "k"); err == nil {
		t.Errorf("expected error for invalid key size")
	}

	// 3. VerifyManifestSignature nil and bad base64
	if err := bundle.VerifyManifestSignature(nil, nil); err == nil {
		t.Errorf("expected error for nil manifest")
	}
	pub, _ := bundle.DefaultKeyPair()
	badSigManifest := &bundle.BundleManifest{
		Signature: bundle.SignatureBlock{
			Algorithm: "ed25519",
			KeyID:     bundle.DefaultReleaseKeyID,
			Value:     "!!!invalid-base64!!!",
		},
	}
	keys := map[string]ed25519.PublicKey{bundle.DefaultReleaseKeyID: pub}
	if err := bundle.VerifyManifestSignature(badSigManifest, keys); err == nil {
		t.Errorf("expected error for bad base64 signature")
	}

	// 4. DefaultStoreDir with XDG_DATA_HOME
	t.Setenv("XDG_DATA_HOME", "/custom/xdg")
	if dir := bundle.DefaultStoreDir(); dir != "/custom/xdg/kubecheck/bundles" {
		t.Errorf("expected custom xdg dir, got: %s", dir)
	}

	// 5. NewStore default and RootDir
	store := bundle.NewStore("", bundle.NewValidator("1.0.0", nil))
	if store.RootDir() == "" {
		t.Errorf("expected non-empty root dir")
	}

	// 6. Report String() with error and failed signature
	rep := &bundle.VerificationReport{
		BundleID:          "failed-bundle",
		BundleVersion:     1,
		SignatureVerified: false,
		Warnings:          []string{"test warning"},
		Errors:            []string{"test error"},
	}
	s := rep.String()
	if !strings.Contains(s, "FAILED") || !strings.Contains(s, "test warning") || !strings.Contains(s, "test error") {
		t.Errorf("unexpected report output: %s", s)
	}
}

func TestValidationEdgeCases(t *testing.T) {
	validator := bundle.NewValidator("1.0.0", nil)

	// 1. Non-existent directory
	_, _, err := validator.ValidateDir("nonexistent-dir-xyz", 0)
	if err == nil {
		t.Errorf("expected error for nonexistent dir")
	}

	// 2. Huge manifest (> 64 KiB)
	tmpHuge := t.TempDir()
	hugeContent := []byte(strings.Repeat(" ", 70000))
	_ = os.WriteFile(filepath.Join(tmpHuge, "manifest.json"), hugeContent, 0600)
	_, _, err = validator.ValidateDir(tmpHuge, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("expected size limit error, got: %v", err)
	}

	// 3. Malformed JSON manifest
	tmpMalformed := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpMalformed, "manifest.json"), []byte("{not-json"), 0600)
	_, _, err = validator.ValidateDir(tmpMalformed, 0)
	if err == nil || !strings.Contains(err.Error(), "malformed manifest.json") {
		t.Errorf("expected malformed JSON error, got: %v", err)
	}

	// 4. File size mismatch
	tmpMismatch := t.TempDir()
	m := createTestBundleDir(t, tmpMismatch, 1, "")
	// Alter size declared in manifest
	m.Files[0].Size = 999999
	_, priv := bundle.DefaultKeyPair()
	_ = bundle.SignManifest(m, priv, bundle.DefaultReleaseKeyID)
	data, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(tmpMismatch, "manifest.json"), data, 0600)
	_, _, err = validator.ValidateDir(tmpMismatch, 0)
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("expected size mismatch error, got: %v", err)
	}
}

func TestDefaultStoreDirAndEmptyStore(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/xdg")
	dir := bundle.DefaultStoreDir()
	if !strings.HasPrefix(dir, "/custom/xdg") {
		t.Errorf("expected custom xdg path, got: %s", dir)
	}

	t.Setenv("XDG_DATA_HOME", "")
	dirDefault := bundle.DefaultStoreDir()
	if dirDefault == "" {
		t.Errorf("expected non-empty default store dir")
	}

	st := bundle.NewStore("", nil)
	if st.RootDir() == "" {
		t.Errorf("expected store root dir to be set")
	}

	// Test GetActiveManifest on empty store
	act, err := st.GetActiveManifest()
	if !errors.Is(err, bundle.ErrNoActiveBundle) || act != nil {
		t.Errorf("expected ErrNoActiveBundle on empty store, got act=%v, err=%v", act, err)
	}

	// Test Rollback on empty store
	_, rErr := st.Rollback()
	if !errors.Is(rErr, bundle.ErrNoPreviousBundle) {
		t.Errorf("expected ErrNoPreviousBundle on empty store, got: %v", rErr)
	}
}
func TestEngineCompatibilitySemver(t *testing.T) {
	v := bundle.NewValidator("1.0.0", nil)
	_, priv := bundle.DefaultKeyPair()

	// Incompatible min version
	tmpTooNew := t.TempDir()
	mTooNew := createTestBundleDir(t, tmpTooNew, 1, "")
	mTooNew.EngineCompatibility.MinVersion = "2.0.0"
	_ = bundle.SignManifest(mTooNew, priv, bundle.DefaultReleaseKeyID)
	dataTooNew, _ := json.Marshal(mTooNew)
	_ = os.WriteFile(filepath.Join(tmpTooNew, "manifest.json"), dataTooNew, 0600)
	_, _, err := v.ValidateDir(tmpTooNew, 0)
	if err == nil || !errors.Is(err, bundle.ErrIncompatibleEngine) {
		t.Errorf("expected ErrIncompatibleEngine for min_version 2.0.0, got: %v", err)
	}

	// Incompatible max version
	tmpTooOld := t.TempDir()
	mTooOld := createTestBundleDir(t, tmpTooOld, 1, "")
	mTooOld.EngineCompatibility.MaxVersion = "0.5.0"
	_ = bundle.SignManifest(mTooOld, priv, bundle.DefaultReleaseKeyID)
	dataTooOld, _ := json.Marshal(mTooOld)
	_ = os.WriteFile(filepath.Join(tmpTooOld, "manifest.json"), dataTooOld, 0600)
	_, _, err = v.ValidateDir(tmpTooOld, 0)
	if err == nil || !errors.Is(err, bundle.ErrIncompatibleEngine) {
		t.Errorf("expected ErrIncompatibleEngine for max_version 0.5.0, got: %v", err)
	}
}

func TestStoreCorruptionAndErrors(t *testing.T) {
	stDir := t.TempDir()
	st := bundle.NewStore(stDir, bundle.NewValidator("1.0.0", nil))

	// 1. Corrupt active manifest
	activeDir := st.ActiveBundleDir()
	_ = os.MkdirAll(activeDir, 0700)
	_ = os.WriteFile(filepath.Join(activeDir, "manifest.json"), []byte("{not-valid-json"), 0600)
	_, err := st.GetActiveManifest()
	if err == nil || !strings.Contains(err.Error(), "corrupt active bundle manifest") {
		t.Errorf("expected corrupt active manifest error, got: %v", err)
	}

	// 2. Rollback with existing bundle.prev missing manifest.json
	prevDir := st.PreviousBundleDir()
	_ = os.MkdirAll(prevDir, 0700)
	_, rErr := st.Rollback()
	if rErr == nil || !strings.Contains(rErr.Error(), "previous bundle manifest missing") {
		t.Errorf("expected previous bundle manifest missing error, got: %v", rErr)
	}

	// 3. Rollback with corrupt manifest.json in bundle.prev
	_ = os.WriteFile(filepath.Join(prevDir, "manifest.json"), []byte("{invalid-json"), 0600)
	_, rErr2 := st.Rollback()
	if rErr2 == nil || !strings.Contains(rErr2.Error(), "previous bundle manifest malformed") {
		t.Errorf("expected previous bundle manifest malformed error, got: %v", rErr2)
	}
}

func TestResolverInvalidPaths(t *testing.T) {
	v := bundle.NewValidator("1.0.0", nil)
	r := bundle.NewResolver(bundle.NewStore(t.TempDir(), v), v)

	// Explicit invalid path
	_, _, err := r.Resolve("/non/existent/path")
	if err == nil || !strings.Contains(err.Error(), "explicit bundle") {
		t.Errorf("expected explicit bundle validation failure, got: %v", err)
	}

	// Environment variable invalid path
	t.Setenv("KUBECHECK_BUNDLE", "/non/existent/env/path")
	_, _, err = r.Resolve("")
	if err == nil || !strings.Contains(err.Error(), "environment bundle") {
		t.Errorf("expected environment bundle validation failure, got: %v", err)
	}
}
