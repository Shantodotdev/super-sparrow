package cli

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shantodotdev/super-sparrow/server/internal/release"
)

// release runs the release-signing subcommands used by the release workflow.
func (r *runner) release(args []string) error {
	if len(args) == 0 {
		return usageError("release needs a subcommand: keygen or sign")
	}

	switch args[0] {
	case "keygen":
		return r.releaseKeygen(args[1:])
	case "sign":
		return r.releaseSign(args[1:])
	}
	return usageError("unknown command release " + args[0])
}

// releaseKeygen prints a new private key on standard output and the public key with its key ID on
// standard error, so the private key can be piped straight into a secret store.
func (r *runner) releaseKeygen(args []string) error {
	if _, err := parse(flag.NewFlagSet("release keygen", flag.ContinueOnError), args, 0); err != nil {
		return err
	}

	public, private, err := release.GenerateKey()
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, release.EncodeKey(private))
	fmt.Fprintf(r.errOut, `Public key: %s
Key ID:     %s

The private key was printed on standard output. Save it now; it is not stored anywhere else.
  - Put the private key in the GitHub "release" environment secret CLI_SIGNING_KEY.
  - Add the public key to release.PublicKeys in server/internal/release/release.go and ship a build
    with it before the first signed release.
`, release.EncodeKey(public), release.KeyID(public))
	return nil
}

// releaseSign writes manifest.json and manifest.json.sig for the sparrow binaries in a directory.
func (r *runner) releaseSign(args []string) error {
	flags := flag.NewFlagSet("release sign", flag.ContinueOnError)
	keyFile := flags.String("key-file", "", "")
	version := flags.String("version", "", "")
	dir := flags.String("dir", "", "")
	if _, err := parse(flags, args, 0); err != nil {
		return err
	}
	if *keyFile == "" || *version == "" || *dir == "" {
		return usageError("release sign needs --key-file (- for standard input), --version, and --dir")
	}

	var keyData []byte
	var err error
	if *keyFile == "-" {
		keyData, err = io.ReadAll(r.stdin)
	} else {
		keyData, err = os.ReadFile(*keyFile)
	}
	if err != nil {
		return fmt.Errorf("read --key-file: %w", err)
	}
	private, err := release.ParsePrivateKey(string(keyData))
	if err != nil {
		return err
	}

	manifest, err := release.BuildManifest(*version, *dir)
	if err != nil {
		return err
	}
	signature, err := release.Sign(manifest, private)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(*dir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath+".sig", signature, 0o644); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}

	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("derive public key")
	}
	fmt.Fprintf(r.out, "Signed %s with key %s: %s\n", *version, release.KeyID(public), manifestPath+" and "+manifestPath+".sig")
	return nil
}
