// Package release signs and verifies the CLI release manifest, and compares release versions.
// The manifest lists every binary of a release with its checksum; one Ed25519 signature over the
// manifest's exact bytes lets `sparrow update` trust files served by any server.
package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PublicKeys are the base64 Ed25519 public keys whose signatures the CLI trusts: the current key
// and a spare. The team adds the public key printed by `sparrow release keygen` here.
var PublicKeys = []string{
	// Key ID 6c7791d57d42d91d, made 2026-11-11. Its private key is the release environment's
	// CLI_SIGNING_KEY secret.
	"Z0Zh2j7GYQi0iF/LYQJC1YSs52pg5aVwZCfUsTpAQys=",
}

// DevVersion is the version of a build that was not made by the release workflow. It is older
// than every release.
const DevVersion = "dev"

// Targets are the platforms a release is built for, as GOOS-GOARCH.
var Targets = []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"}

// Manifest describes one release. Its serialized bytes are what the signature covers.
type Manifest struct {
	Version string          `json:"version"`
	Files   map[string]File `json:"files"`
}

// File is one downloadable binary of a release.
type File struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Signature is the content of manifest.json.sig.
type Signature struct {
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

var (
	versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	// runningVersionPattern also accepts the pre-release and build suffixes of a development build,
	// such as v1.2.3-dirty, v1.2.3-rc.1, or v1.2.3+abc.
	runningVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
	hashPattern           = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// signingDomain is prepended to the manifest bytes before signing, so a signature made for a
// manifest cannot be mistaken for a signature over any other kind of data.
const signingDomain = "sparrow-cli-manifest-v1\n"

// signedPayload returns the exact bytes that are signed and verified for a manifest.
func signedPayload(manifest []byte) []byte {
	return append([]byte(signingDomain), manifest...)
}

// GenerateKey makes a new signing key pair.
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("generate signing key: %w", err)
	}
	return public, private, nil
}

// KeyID returns the identifier of a public key: the first 8 bytes of its SHA-256 hash, in hex.
func KeyID(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:8])
}

// EncodeKey returns key in base64, the form used on the command line and in PublicKeys.
func EncodeKey(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

// ParsePublicKey decodes a base64 Ed25519 public key.
func ParsePublicKey(encoded string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("a public key must be a base64 Ed25519 key of 32 bytes")
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodes a base64 Ed25519 private key, either the 64-byte form that keygen
// prints or a 32-byte seed.
func ParsePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("the private key is not valid base64")
	}
	switch len(raw) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	}
	return nil, errors.New("the private key must be an Ed25519 key of 64 bytes, or a 32-byte seed")
}

// Sign signs the manifest bytes, prefixed with a domain separator, and returns the content of manifest.json.sig.
func Sign(manifest []byte, private ed25519.PrivateKey) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, errors.New("sign manifest: invalid private key")
	}

	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("sign manifest: invalid private key")
	}
	signature, err := json.Marshal(Signature{
		KeyID:     KeyID(public),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, signedPayload(manifest))),
	})
	if err != nil {
		return nil, fmt.Errorf("encode signature: %w", err)
	}
	return append(signature, '\n'), nil
}

// Verify checks the signature file against the manifest bytes using the trusted publicKeys
// (base64 Ed25519 keys) and returns the parsed manifest. It fails when no key is configured, the
// signature names an unknown key, the signature does not match, or the manifest is malformed.
func Verify(manifest, signatureFile []byte, publicKeys []string) (*Manifest, error) {
	if len(publicKeys) == 0 {
		return nil, errors.New("this build of sparrow has no release signing key, so it cannot verify updates; download a release from GitHub instead")
	}

	var signature Signature
	if err := json.Unmarshal(signatureFile, &signature); err != nil {
		return nil, fmt.Errorf("read signature file: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(signature.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return nil, errors.New("the signature file does not hold a valid Ed25519 signature")
	}

	var trusted ed25519.PublicKey
	for _, encoded := range publicKeys {
		public, err := ParsePublicKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("built-in release key: %w", err)
		}
		if KeyID(public) == signature.KeyID {
			trusted = public
			break
		}
	}
	if trusted == nil {
		return nil, fmt.Errorf("the release is signed with unknown key %q; this build of sparrow does not trust it", signature.KeyID)
	}
	if !ed25519.Verify(trusted, signedPayload(manifest), raw) {
		return nil, errors.New("the manifest signature is invalid; the release may have been tampered with")
	}

	return parseManifest(manifest)
}

// parseManifest decodes and validates manifest bytes.
func parseManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if !versionPattern.MatchString(manifest.Version) {
		return nil, fmt.Errorf("manifest version %q must look like v1.2.3", manifest.Version)
	}
	for target, file := range manifest.Files {
		name := file.Name
		if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
			return nil, fmt.Errorf("manifest file name %q for %s must be a plain file name", name, target)
		}
		if !hashPattern.MatchString(file.SHA256) {
			return nil, fmt.Errorf("manifest checksum for %s must be 64 lowercase hex characters", target)
		}
		if file.Size <= 0 {
			return nil, fmt.Errorf("manifest size for %s must be positive", target)
		}
	}
	return &manifest, nil
}

// BuildManifest finds the sparrow-<os>-<arch>[.exe] binaries in dir and returns the manifest bytes
// for version. Every target must have a binary; the error names the missing ones.
func BuildManifest(version, dir string) ([]byte, error) {
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("version %q must look like v1.2.3", version)
	}

	manifest := Manifest{Version: version, Files: map[string]File{}}
	var missing []string
	for _, target := range Targets {
		name := "sparrow-" + target
		if strings.HasPrefix(target, "windows-") {
			name += ".exe"
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		manifest.Files[target] = File{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s is missing %s", dir, strings.Join(missing, ", "))
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return append(data, '\n'), nil
}

// CompareVersions returns -1, 0, or 1 as a is older than, equal to, or newer than b. Versions look
// like v1.2.3, optionally with a pre-release (-rc.1, -dirty) and build metadata (+abc). A
// pre-release is older than its release, build metadata is ignored, and DevVersion is older than
// every release.
func CompareVersions(a, b string) (int, error) {
	if a == DevVersion || b == DevVersion {
		switch {
		case a == b:
			return 0, nil
		case a == DevVersion:
			return -1, nil
		}
		return 1, nil
	}

	left, leftPre, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	right, rightPre, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range left {
		if left[i] != right[i] {
			if left[i] < right[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return comparePreRelease(leftPre, rightPre), nil
}

// comparePreRelease orders pre-release strings by semantic versioning: no pre-release is newer
// than any pre-release; otherwise dot-separated identifiers compare one by one, numbers
// numerically and below text, and a shorter list is older when the rest is equal.
func comparePreRelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		leftNumber, leftErr := strconv.ParseUint(left[i], 10, 64)
		rightNumber, rightErr := strconv.ParseUint(right[i], 10, 64)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			return 1
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		}
		return strings.Compare(left[i], right[i])
	}
	return cmpInts(len(left), len(right))
}

// cmpInts returns -1, 0, or 1 as a is less than, equal to, or greater than b.
func cmpInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// parseVersion returns the major, minor, and patch numbers and the pre-release text of a version.
func parseVersion(version string) ([3]int, string, error) {
	match := runningVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return [3]int{}, "", fmt.Errorf("version %q must look like v1.2.3 or %s", version, DevVersion)
	}

	var numbers [3]int
	for i := range numbers {
		number, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, "", fmt.Errorf("version %q is out of range: %w", version, err)
		}
		numbers[i] = number
	}
	return numbers, match[4], nil
}
