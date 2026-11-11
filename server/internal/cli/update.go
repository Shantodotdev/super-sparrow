package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/release"
)

const (
	manifestLimit   = 1 << 20
	downloadTimeout = 10 * time.Minute
)

// update replaces the running program with the newest signed release the configured server offers.
func (r *runner) update(ctx context.Context, args []string) error {
	if _, err := parse(flag.NewFlagSet("update", flag.ContinueOnError), args, 0); err != nil {
		return err
	}

	// Updating needs only the server's address, not a signed-in session: release files are public.
	rawURL := r.env.Getenv("SPARROW_URL")
	if strings.TrimSpace(rawURL) == "" {
		creds, err := r.env.store().load()
		if err != nil {
			return errors.New("update needs the server's address: run `sparrow login --url URL` or set SPARROW_URL")
		}
		rawURL = creds.URL
	}
	baseURL, err := parseServerURL(rawURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Fetch the signed manifest and trust nothing in it until the signature verifies.
	manifestBytes, err := r.fetch(ctx, baseURL, "/cli/manifest.json", manifestLimit)
	if err != nil {
		return err
	}
	signatureBytes, err := r.fetch(ctx, baseURL, "/cli/manifest.json.sig", manifestLimit)
	if err != nil {
		return err
	}
	manifest, err := release.Verify(manifestBytes, signatureBytes, r.env.PublicKeys)
	if err != nil {
		return err
	}

	order, err := release.CompareVersions(manifest.Version, r.env.Version)
	if err != nil {
		return fmt.Errorf("compare versions: %w", err)
	}
	if order <= 0 {
		fmt.Fprintf(r.out, "sparrow %s is up to date (the server offers %s).\n", r.env.Version, manifest.Version)
		return nil
	}

	target := runtime.GOOS + "-" + runtime.GOARCH
	file, found := manifest.Files[target]
	if !found {
		return fmt.Errorf("release %s has no build for %s", manifest.Version, target)
	}

	// Download exactly the size and checksum the signed manifest names.
	binary, err := r.fetch(ctx, baseURL, "/cli/"+url.PathEscape(file.Name), file.Size)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(binary)
	if int64(len(binary)) != file.Size || hex.EncodeToString(sum[:]) != file.SHA256 {
		return fmt.Errorf("downloaded %s does not match the signed manifest (size or checksum differs); nothing was changed", file.Name)
	}

	executable, err := r.env.Executable()
	if err != nil {
		return fmt.Errorf("find this program: %w", err)
	}
	if err := replaceExecutable(executable, binary, runtime.GOOS); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Updated sparrow from %s to %s.\n", r.env.Version, manifest.Version)
	return nil
}

// fetch downloads path from the server. A body larger than limit bytes is an error.
func (r *runner) fetch(ctx context.Context, baseURL *url.URL, path string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.JoinPath(path).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpClient := *r.env.HTTPClient
	httpClient.CheckRedirect = refuseInsecureRedirect
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d (does this server publish CLI releases?)", path, response.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than the %d bytes expected", path, limit)
	}
	return data, nil
}

// refuseInsecureRedirect stops a redirect that would move a download from https to plain http,
// and otherwise allows the default limit of ten redirects.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect from https to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// removeOldExecutable deletes the path.old file a previous update on Windows left behind, which
// could not be removed while the old program was still running. Failures are ignored: the file is
// only clutter, and the next update removes it again.
func removeOldExecutable(executable, goos string) {
	if goos == "windows" {
		os.Remove(executable + ".old")
	}
}

// replaceExecutable atomically puts data in place of the program at path: it writes a temporary
// file in the same directory and renames it over the original. Windows cannot replace a running
// program, so there the running file is renamed to path.old first.
func replaceExecutable(path string, data []byte, goos string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".sparrow-update-*")
	if err != nil {
		return fmt.Errorf("write the update next to %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write the update: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("flush the update to disk: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write the update: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("make the update executable: %w", err)
	}

	if goos != "windows" {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("replace %s: %w", path, err)
		}
		return nil
	}

	old := path + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the previous %s: %w", old, err)
	}
	if err := os.Rename(path, old); err != nil {
		return fmt.Errorf("move the running program aside: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		if restoreErr := os.Rename(old, path); restoreErr != nil {
			return errors.Join(fmt.Errorf("replace %s: %w", path, err), restoreErr)
		}
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
