package bundle

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrIncompatibleFormat = errors.New("unsupported bundle format version")
	ErrIncompatibleEngine = errors.New("engine version is incompatible with bundle compatibility range")
	ErrRollbackProhibited = errors.New("rollback attempt detected: incoming bundle version is lower than active version")
	ErrBundleExpired      = errors.New("bundle is expired and cannot be loaded")
)

// VerificationReport captures detailed status for each stage of bundle validation per UPDATE-SIGN-REQ-005.
type VerificationReport struct {
	BundleID          string   `json:"bundle_id"`
	BundleVersion     int      `json:"bundle_version"`
	ManifestValid     bool     `json:"manifest_valid"`
	EngineCompatible  bool     `json:"engine_compatible"`
	FilesChecked      int      `json:"files_checked"`
	FilesPassed       int      `json:"files_passed"`
	SignatureVerified bool     `json:"signature_verified"`
	IsFresh           bool     `json:"is_fresh"`
	Warnings          []string `json:"warnings,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

func (r *VerificationReport) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge Bundle: %s (v%d)\n", r.BundleID, r.BundleVersion))
	sb.WriteString(fmt.Sprintf("  [Checksums] %d/%d file hashes matched\n", r.FilesPassed, r.FilesChecked))
	if r.SignatureVerified {
		sb.WriteString("  [Signature] Cryptographic Ed25519 signature: VERIFIED\n")
	} else {
		sb.WriteString("  [Signature] Cryptographic Ed25519 signature: FAILED\n")
	}
	if len(r.Warnings) > 0 {
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("  [Warning]   %s\n", w))
		}
	}
	if len(r.Errors) > 0 {
		for _, e := range r.Errors {
			sb.WriteString(fmt.Sprintf("  [Error]     %s\n", e))
		}
	}
	return sb.String()
}

// Validator validates knowledge bundle directories against all five normative stages.
type Validator struct {
	engineVersion string
	trustedKeys   map[string]ed25519.PublicKey
	currentTime   time.Time
}

// NewValidator initializes a Validator.
func NewValidator(engineVersion string, trustedKeys map[string]ed25519.PublicKey) *Validator {
	if trustedKeys == nil {
		trustedKeys = make(map[string]ed25519.PublicKey)
		pub, _ := DefaultKeyPair()
		trustedKeys[DefaultReleaseKeyID] = pub
	}
	return &Validator{
		engineVersion: engineVersion,
		trustedKeys:   trustedKeys,
		currentTime:   time.Now().UTC(),
	}
}

// SetTime allows deterministic time injection for testing freshness policies.
func (v *Validator) SetTime(t time.Time) {
	v.currentTime = t
}

// ValidateDir validates an extracted bundle directory.
func (v *Validator) ValidateDir(bundleDir string, activeBundleVersion int) (*BundleManifest, *VerificationReport, error) {
	report := &VerificationReport{}

	// Stage 1: Load and check manifest.json
	manifestPath := filepath.Join(bundleDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("failed to read manifest.json: %v", err))
		return nil, report, err
	}

	if len(manifestData) > 65536 {
		err := errors.New("manifest.json exceeds maximum allowed size of 64 KiB")
		report.Errors = append(report.Errors, err.Error())
		return nil, report, err
	}

	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		wErr := fmt.Errorf("malformed manifest.json: %w", err)
		report.Errors = append(report.Errors, wErr.Error())
		return nil, report, wErr
	}

	report.BundleID = manifest.BundleID
	report.BundleVersion = manifest.BundleVersion
	report.ManifestValid = true

	// Check format_version
	if manifest.FormatVersion != CurrentFormatVersion {
		err := fmt.Errorf("%w: bundle requires format %q, engine supports %q", ErrIncompatibleFormat, manifest.FormatVersion, CurrentFormatVersion)
		report.Errors = append(report.Errors, err.Error())
		return nil, report, err
	}

	// Stage 2: Engine compatibility
	if err := checkEngineCompatibility(manifest.EngineCompatibility, v.engineVersion); err != nil {
		report.Errors = append(report.Errors, err.Error())
		return nil, report, err
	}
	report.EngineCompatible = true

	// Stage 3: File content checksum integrity
	report.FilesChecked = len(manifest.Files)
	for _, f := range manifest.Files {
		fullPath := filepath.Join(bundleDir, f.Path)
		observedHash, observedSize, err := ComputeFileSHA256(fullPath)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("missing bundle file %q: %v", f.Path, err))
			return nil, report, fmt.Errorf("missing bundle file %q: %w", f.Path, err)
		}

		if observedHash != f.SHA256 {
			report.Errors = append(report.Errors, fmt.Sprintf("checksum mismatch on %q: expected %s, got %s", f.Path, f.SHA256, observedHash))
			return nil, report, fmt.Errorf("%w on %q", ErrChecksumMismatch, f.Path)
		}

		if observedSize != f.Size {
			report.Errors = append(report.Errors, fmt.Sprintf("size mismatch on %q: expected %d bytes, got %d", f.Path, f.Size, observedSize))
			return nil, report, fmt.Errorf("%w on %q (size mismatch)", ErrChecksumMismatch, f.Path)
		}

		report.FilesPassed++
	}

	// Stage 4: Cryptographic digital signature verification
	if err := VerifyManifestSignature(&manifest, v.trustedKeys); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("digital signature invalid: %v", err))
		return nil, report, err
	}
	report.SignatureVerified = true

	// Stage 5: Anti-rollback & freshness check
	if activeBundleVersion > 0 && manifest.BundleVersion < activeBundleVersion {
		err := fmt.Errorf("%w: incoming bundle version %d < active bundle version %d", ErrRollbackProhibited, manifest.BundleVersion, activeBundleVersion)
		report.Errors = append(report.Errors, err.Error())
		return nil, report, err
	}

	// Freshness evaluation
	report.IsFresh = true
	if manifest.Generation.FreshUntil != "" {
		freshUntil, err := time.Parse(time.RFC3339, manifest.Generation.FreshUntil)
		if err == nil {
			if v.currentTime.After(freshUntil) {
				report.IsFresh = false
				report.Warnings = append(report.Warnings, fmt.Sprintf("bundle knowledge expired on %s; may lack recent Kubernetes security rules", manifest.Generation.FreshUntil))

				// If older than 2 years past fresh_until, reject as expired per UPDATE-SIGN-REQ-027
				twoYearsPast := freshUntil.AddDate(2, 0, 0)
				if v.currentTime.After(twoYearsPast) {
					err := fmt.Errorf("%w: bundle expired on %s (> 2 years past freshness limit)", ErrBundleExpired, manifest.Generation.FreshUntil)
					report.Errors = append(report.Errors, err.Error())
					return nil, report, err
				}
			}
		}
	}

	return &manifest, report, nil
}

// checkEngineCompatibility verifies that minVersion <= engineVersion <= maxVersion.
func checkEngineCompatibility(compat EngineCompatibility, engineVersion string) error {
	cleanEngine := strings.TrimPrefix(strings.Split(engineVersion, "-")[0], "v")

	if compat.MinVersion != "" {
		if compareSemver(cleanEngine, compat.MinVersion) < 0 {
			return fmt.Errorf("%w: engine %s < required min %s", ErrIncompatibleEngine, cleanEngine, compat.MinVersion)
		}
	}
	if compat.MaxVersion != "" {
		if compareSemver(cleanEngine, compat.MaxVersion) > 0 {
			return fmt.Errorf("%w: engine %s > maximum supported %s", ErrIncompatibleEngine, cleanEngine, compat.MaxVersion)
		}
	}
	return nil
}

// compareSemver compares two simple semver strings (e.g. "1.0.0" and "0.9.1").
func compareSemver(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		n1, _ := strconv.Atoi(parts1[i])
		n2, _ := strconv.Atoi(parts2[i])
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	if len(parts1) < len(parts2) {
		return -1
	}
	if len(parts1) > len(parts2) {
		return 1
	}
	return 0
}
