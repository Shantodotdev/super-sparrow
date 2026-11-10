package release

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

const goodManifest = `{"version":"v1.2.3","files":{"linux-amd64":{"name":"sparrow-linux-amd64","sha256":"` +
	"0000000000000000000000000000000000000000000000000000000000000000" + `","size":10}}}`

func TestSignAndVerify(t *testing.T) {
	public, private := testKey(t)
	signature, err := Sign([]byte(goodManifest), private)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := Verify([]byte(goodManifest), signature, []string{EncodeKey(public)})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "v1.2.3" || manifest.Files["linux-amd64"].Size != 10 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestVerifyRejects(t *testing.T) {
	public, private := testKey(t)
	otherPublic, otherPrivate := testKey(t)
	signature, err := Sign([]byte(goodManifest), private)
	if err != nil {
		t.Fatal(err)
	}
	otherSignature, err := Sign([]byte(goodManifest), otherPrivate)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{EncodeKey(public)}

	tests := []struct {
		name      string
		manifest  string
		signature []byte
		keys      []string
		want      string
	}{
		{"tampered manifest", strings.Replace(goodManifest, "v1.2.3", "v9.9.9", 1), signature, keys, "tampered"},
		{"unknown key id", goodManifest, otherSignature, keys, "unknown key"},
		{"no keys built in", goodManifest, signature, nil, "no release signing key"},
		{"garbage signature file", goodManifest, []byte("nope"), keys, "read signature file"},
		{"wrong key under a known id", goodManifest, wrongKeySignature(t, public, otherPrivate), keys, "tampered"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Verify([]byte(test.manifest), test.signature, test.keys)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	// A spare key in the list is accepted.
	if _, err := Verify([]byte(goodManifest), otherSignature, append(keys, EncodeKey(otherPublic))); err != nil {
		t.Fatalf("spare key: %v", err)
	}
}

// wrongKeySignature signs with private but claims to be the key public.
func wrongKeySignature(t *testing.T, public ed25519.PublicKey, private ed25519.PrivateKey) []byte {
	t.Helper()
	signature, err := Sign([]byte(goodManifest), private)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.Replace(string(signature), KeyID(private.Public().(ed25519.PublicKey)), KeyID(public), 1))
}

func TestVerifyRejectsUnsafeManifests(t *testing.T) {
	public, private := testKey(t)
	for name, manifest := range map[string]string{
		"path in file name": strings.Replace(goodManifest, "sparrow-linux-amd64", "../evil", 1),
		"bad version":       strings.Replace(goodManifest, "v1.2.3", "1.2", 1),
		"bad checksum":      strings.Replace(goodManifest, "0000000000000000000000000000000000000000000000000000000000000000", "xyz", 1),
	} {
		signature, err := Sign([]byte(manifest), private)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Verify([]byte(manifest), signature, []string{EncodeKey(public)}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"dev", "v0.0.1", -1},
		{"v0.0.1", "dev", 1},
		{"dev", "dev", 0},
		{"v1.2.3-dirty", "v1.2.3", -1},
		{"v1.2.3", "v1.2.3-rc.1", 1},
		{"v1.2.3-rc.1", "v1.2.3-rc.2", -1},
		{"v1.2.3-rc.2", "v1.2.3-rc.10", -1},
		{"v1.2.3-rc", "v1.2.3-rc.1", -1},
		{"v1.2.3-1", "v1.2.3-alpha", -1},
		{"v1.2.3+abc", "v1.2.3+def", 0},
		{"v1.2.3-dirty+abc", "v1.2.4", -1},
		{"v1.2.4-dirty", "v1.2.3", 1},
	}
	for _, test := range tests {
		got, err := CompareVersions(test.a, test.b)
		if err != nil || got != test.want {
			t.Errorf("CompareVersions(%s, %s) = %d, %v; want %d", test.a, test.b, got, err, test.want)
		}
	}
	if _, err := CompareVersions("1.2.3", "v1.2.3"); err == nil {
		t.Error("accepted a version without v")
	}
}

func TestKeyIDAndParsing(t *testing.T) {
	public, private := testKey(t)
	if len(KeyID(public)) != 16 {
		t.Fatalf("key id %q", KeyID(public))
	}
	parsed, err := ParsePrivateKey(EncodeKey(private))
	if err != nil || !parsed.Equal(private) {
		t.Fatalf("private key round trip: %v", err)
	}
	fromSeed, err := ParsePrivateKey(EncodeKey(private.Seed()))
	if err != nil || !fromSeed.Equal(private) {
		t.Fatalf("seed round trip: %v", err)
	}
	if _, err := ParsePublicKey("AAAA"); err == nil {
		t.Error("accepted a short public key")
	}
}

func TestBuildManifest(t *testing.T) {
	dir := t.TempDir()
	for _, target := range Targets {
		name := "sparrow-" + target
		if strings.HasPrefix(target, "windows-") {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(target), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	data, err := BuildManifest("v1.0.0", dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := parseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != len(Targets) || manifest.Files["windows-amd64"].Name != "sparrow-windows-amd64.exe" || manifest.Files["linux-amd64"].Size != int64(len("linux-amd64")) {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestBuildManifestNamesMissingTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sparrow-linux-amd64"), []byte("linux"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := BuildManifest("v1.0.0", dir)
	if err == nil || !strings.Contains(err.Error(), "sparrow-darwin-arm64") || !strings.Contains(err.Error(), "sparrow-windows-arm64.exe") || strings.Contains(err.Error(), "sparrow-linux-amd64,") {
		t.Fatalf("error = %v", err)
	}
	if _, err := BuildManifest("v1.0.0", t.TempDir()); err == nil {
		t.Error("built a manifest without binaries")
	}
}

func TestSignatureIsBoundToTheManifestDomain(t *testing.T) {
	public, private := testKey(t)

	// A signature over the bare manifest bytes, as an older signer would make, is refused.
	raw := ed25519.Sign(private, []byte(goodManifest))
	file := `{"key_id":"` + KeyID(public) + `","signature":"` + base64.StdEncoding.EncodeToString(raw) + `"}`
	if _, err := Verify([]byte(goodManifest), []byte(file), []string{EncodeKey(public)}); err == nil {
		t.Fatal("accepted a signature without the domain separator")
	}
}
