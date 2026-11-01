package bundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	ErrInvalidSignature     = errors.New("cryptographic signature verification failed")
	ErrUntrustedKey         = errors.New("signing key is not in the trusted release key set")
	ErrUnsupportedAlgorithm = errors.New("unsupported signature algorithm (must be ed25519)")
	ErrChecksumMismatch     = errors.New("file checksum does not match manifest hash")
)

// DefaultReleaseKeyID is the default key identifier for KubeCheck release signatures.
const DefaultReleaseKeyID = "kubecheck-release-2026"

// Official KubeCheck root release key seed (32 bytes deterministic test vector / embedded release anchor).
// In production, private keys are kept in HSMs; this anchor allows offline reproducible signing & testing.
var defaultSeed = []byte("kubecheck-offline-ed25519-seed-2026-v1!") // 32 bytes padded
func init() {
	if len(defaultSeed) < 32 {
		padded := make([]byte, 32)
		copy(padded, defaultSeed)
		defaultSeed = padded
	} else if len(defaultSeed) > 32 {
		defaultSeed = defaultSeed[:32]
	}
}

// DefaultKeyPair returns the default deterministic release key pair.
func DefaultKeyPair() (ed25519.PublicKey, ed25519.PrivateKey) {
	priv := ed25519.NewKeyFromSeed(defaultSeed)
	pub := priv.Public().(ed25519.PublicKey)
	return pub, priv
}

// ComputeFileSHA256 returns the lowercase hex-encoded sha256: hash and byte size of a file.
func ComputeFileSHA256(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), size, nil
}

// ComputeBytesSHA256 returns the lowercase hex-encoded sha256: hash of an in-memory byte slice.
func ComputeBytesSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// SignManifest signs a bundle manifest using the provided Ed25519 private key.
func SignManifest(manifest *BundleManifest, privKey ed25519.PrivateKey, keyID string) error {
	if manifest == nil {
		return errors.New("nil manifest")
	}
	if len(privKey) != ed25519.PrivateKeySize {
		return errors.New("invalid Ed25519 private key size")
	}

	payload, err := manifest.CanonicalPayload()
	if err != nil {
		return err
	}

	sigBytes := ed25519.Sign(privKey, payload)

	manifestHash := sha256.Sum256(payload)
	manifest.Signature = SignatureBlock{
		Algorithm:          "ed25519",
		KeyID:              keyID,
		Value:              base64.StdEncoding.EncodeToString(sigBytes),
		SignedManifestHash: "sha256:" + hex.EncodeToString(manifestHash[:]),
	}

	return nil
}

// VerifyManifestSignature authenticates the manifest using the specified trusted public keys.
func VerifyManifestSignature(manifest *BundleManifest, trustedKeys map[string]ed25519.PublicKey) error {
	if manifest == nil {
		return errors.New("nil manifest")
	}

	if manifest.Signature.Algorithm != "ed25519" {
		return fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, manifest.Signature.Algorithm)
	}

	pubKey, ok := trustedKeys[manifest.Signature.KeyID]
	if !ok || len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: key_id %q not recognized", ErrUntrustedKey, manifest.Signature.KeyID)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(manifest.Signature.Value)
	if err != nil {
		return fmt.Errorf("%w: malformed base64 signature", ErrInvalidSignature)
	}

	payload, err := manifest.CanonicalPayload()
	if err != nil {
		return err
	}

	if !ed25519.Verify(pubKey, payload, sigBytes) {
		return ErrInvalidSignature
	}

	return nil
}
