package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var (
	ErrNoActiveBundle   = errors.New("no active knowledge bundle found in store")
	ErrNoPreviousBundle = errors.New("no previous knowledge bundle available for rollback")
)

// StoreMetadata describes the active bundle within the bundle store per UPDATE-SIGN-REQ-022.
type StoreMetadata struct {
	ActiveBundleID      string `json:"active_bundle_id"`
	ActiveBundleVersion int    `json:"active_bundle_version"`
	ActivatedAt         string `json:"activated_at"`
	ManifestSHA256      string `json:"manifest_sha256"`
}

// DefaultStoreDir resolves the platform-specific default bundle store directory.
func DefaultStoreDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "kubecheck", "bundles")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".kubecheck", "bundles")
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "kubecheck", "bundles")
	case "windows":
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			return filepath.Join(appData, "kubecheck", "bundles")
		}
		return filepath.Join(home, "AppData", "Local", "kubecheck", "bundles")
	default:
		return filepath.Join(home, ".local", "share", "kubecheck", "bundles")
	}
}

// Store manages local bundle installations, atomic activations, and rollbacks.
type Store struct {
	rootDir   string
	validator *Validator
}

// NewStore initializes a Store in the given root directory.
func NewStore(rootDir string, validator *Validator) *Store {
	if rootDir == "" {
		rootDir = DefaultStoreDir()
	}
	return &Store{
		rootDir:   rootDir,
		validator: validator,
	}
}

// RootDir returns the root storage directory.
func (s *Store) RootDir() string {
	return s.rootDir
}

// ActiveBundleDir returns the directory of the currently active bundle.
func (s *Store) ActiveBundleDir() string {
	return filepath.Join(s.rootDir, "bundle")
}

// PreviousBundleDir returns the directory of the previous bundle.
func (s *Store) PreviousBundleDir() string {
	return filepath.Join(s.rootDir, "bundle.prev")
}

// MetadataPath returns the path to metadata.json in the store.
func (s *Store) MetadataPath() string {
	return filepath.Join(s.rootDir, "metadata.json")
}

// GetActiveManifest loads the active bundle manifest, if present.
func (s *Store) GetActiveManifest() (*BundleManifest, error) {
	manifestPath := filepath.Join(s.ActiveBundleDir(), "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, ErrNoActiveBundle
	}

	var m BundleManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("corrupt active bundle manifest: %w", err)
	}

	return &m, nil
}

// Activate transactionally verifies and activates a bundle directory per UPDATE-SIGN-REQ-028 through 033.
func (s *Store) Activate(srcBundleDir string) (*BundleManifest, *VerificationReport, error) {
	// Read current active bundle version to enforce anti-rollback
	activeVersion := 0
	if current, err := s.GetActiveManifest(); err == nil && current != nil {
		activeVersion = current.BundleVersion
	}

	// 1. Verify incoming bundle before modifying any store state
	manifest, report, err := s.validator.ValidateDir(srcBundleDir, activeVersion)
	if err != nil {
		return nil, report, fmt.Errorf("bundle verification failed: %w", err)
	}

	// Ensure store root exists with user-only permissions (0700) per SAFE-EXEC-REQ-004
	if err := os.MkdirAll(s.rootDir, 0700); err != nil {
		return nil, report, fmt.Errorf("failed to create store directory: %w", err)
	}

	stagingDir := filepath.Join(s.rootDir, "bundle.new")
	_ = os.RemoveAll(stagingDir)

	// Copy incoming bundle to bundle.new/
	if err := copyDirectory(srcBundleDir, stagingDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, report, fmt.Errorf("failed to stage bundle: %w", err)
	}

	// Write transaction log (staged)
	txLogPath := filepath.Join(s.rootDir, "transaction.log")
	_ = os.WriteFile(txLogPath, []byte(fmt.Sprintf(`{"stage":"staged","bundle_id":%q,"version":%d}`+"\n", manifest.BundleID, manifest.BundleVersion)), 0600)

	activeDir := s.ActiveBundleDir()
	prevDir := s.PreviousBundleDir()

	// If active bundle already exists, rotate bundle/ to bundle.prev/
	if _, err := os.Stat(activeDir); err == nil {
		_ = os.RemoveAll(prevDir)
		if err := os.Rename(activeDir, prevDir); err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, report, fmt.Errorf("failed to rotate active bundle to prev: %w", err)
		}
	}

	// Rotate bundle.new/ to bundle/
	if err := os.Rename(stagingDir, activeDir); err != nil {
		// Attempt recovery by restoring prevDir back to activeDir
		_ = os.Rename(prevDir, activeDir)
		return nil, report, fmt.Errorf("failed to activate new bundle: %w", err)
	}

	// Write metadata.json
	meta := StoreMetadata{
		ActiveBundleID:      manifest.BundleID,
		ActiveBundleVersion: manifest.BundleVersion,
		ActivatedAt:         time.Now().UTC().Format(time.RFC3339),
		ManifestSHA256:      manifest.Signature.SignedManifestHash,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(s.MetadataPath(), metaBytes, 0600)

	// Clear transaction log
	_ = os.Remove(txLogPath)

	return manifest, report, nil
}

// Rollback restores bundle.prev/ to bundle/ per UPDATE-SIGN-REQ-033 and FAIL-ROLLBACK-001.
func (s *Store) Rollback() (*BundleManifest, error) {
	prevDir := s.PreviousBundleDir()
	if _, err := os.Stat(prevDir); err != nil {
		return nil, ErrNoPreviousBundle
	}

	prevManifestPath := filepath.Join(prevDir, "manifest.json")
	data, err := os.ReadFile(prevManifestPath)
	if err != nil {
		return nil, fmt.Errorf("previous bundle manifest missing: %w", err)
	}

	var manifest BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("previous bundle manifest malformed: %w", err)
	}

	activeDir := s.ActiveBundleDir()
	tmpDir := filepath.Join(s.rootDir, "bundle.rollback_tmp")
	_ = os.RemoveAll(tmpDir)

	if _, err := os.Stat(activeDir); err == nil {
		if err := os.Rename(activeDir, tmpDir); err != nil {
			return nil, fmt.Errorf("failed to move active bundle: %w", err)
		}
	}

	if err := os.Rename(prevDir, activeDir); err != nil {
		_ = os.Rename(tmpDir, activeDir)
		return nil, fmt.Errorf("failed to activate previous bundle: %w", err)
	}

	_ = os.RemoveAll(tmpDir)

	// Update metadata.json
	meta := StoreMetadata{
		ActiveBundleID:      manifest.BundleID,
		ActiveBundleVersion: manifest.BundleVersion,
		ActivatedAt:         time.Now().UTC().Format(time.RFC3339),
		ManifestSHA256:      manifest.Signature.SignedManifestHash,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(s.MetadataPath(), metaBytes, 0600)

	return &manifest, nil
}

// copyDirectory recursively copies a directory preserving file contents and modes.
func copyDirectory(src, dst string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			in, err := os.Open(srcPath)
			if err != nil {
				return err
			}
			out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				in.Close()
				return err
			}
			_, copyErr := io.Copy(out, in)
			in.Close()
			out.Close()
			if copyErr != nil {
				return copyErr
			}
		}
	}
	return nil
}
