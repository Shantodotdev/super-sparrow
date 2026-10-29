package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	r2Region       = "auto"
	s3Service      = "s3"
	r2Timeout      = 10 * time.Minute
	emptyHash      = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	maxErrorDetail = 4 << 10
)

// Bucket is an S3-compatible bucket, such as a Cloudflare R2 bucket, reached with path-style
// requests signed with AWS Signature Version 4.
type Bucket struct {
	endpoint        *url.URL
	bucket          string
	region          string
	accessKeyID     string
	secretAccessKey string
	http            *http.Client
	now             func() time.Time
}

// Object is one object listed in a bucket.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// NewR2Bucket returns the R2 bucket of a Cloudflare account.
func NewR2Bucket(accountID, accessKeyID, secretAccessKey, bucket string) *Bucket {
	endpoint := &url.URL{Scheme: "https", Host: accountID + ".r2.cloudflarestorage.com"}
	return newBucket(endpoint, r2Region, accessKeyID, secretAccessKey, bucket, time.Now)
}

func newBucket(endpoint *url.URL, region, accessKeyID, secretAccessKey, bucket string, now func() time.Time) *Bucket {
	return &Bucket{
		endpoint: endpoint, bucket: bucket, region: region, accessKeyID: accessKeyID,
		secretAccessKey: secretAccessKey, http: &http.Client{Timeout: r2Timeout}, now: now,
	}
}

// Put uploads the file at path as key. The bucket replaces an existing object only once the new
// upload has completed, so a failed upload leaves the old copy in place.
func (b *Bucket) Put(ctx context.Context, key, path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return 0, fmt.Errorf("hash %s: %w", path, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewind %s: %w", path, err)
	}

	request, err := b.newRequest(ctx, http.MethodPut, key, nil, file)
	if err != nil {
		return 0, err
	}
	request.ContentLength = size
	request.Header.Set("Content-Type", "application/octet-stream")

	response, err := b.send(request, hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		return 0, fmt.Errorf("upload %s: %w", key, err)
	}
	response.Body.Close()
	return size, nil
}

// Get downloads key. The caller closes the returned body.
func (b *Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	request, err := b.newRequest(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}

	response, err := b.send(request, emptyHash)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", key, err)
	}
	return response.Body, nil
}

// Delete removes key.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	request, err := b.newRequest(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}

	response, err := b.send(request, emptyHash)
	if err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	response.Body.Close()
	return nil
}

// List returns every object whose key starts with prefix, in key order.
func (b *Bucket) List(ctx context.Context, prefix string) ([]Object, error) {
	var objects []Object
	token := ""
	for {
		query := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			query.Set("continuation-token", token)
		}

		request, err := b.newRequest(ctx, http.MethodGet, "", query, nil)
		if err != nil {
			return nil, err
		}
		response, err := b.send(request, emptyHash)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}

		var page struct {
			Contents []struct {
				Key          string    `xml:"Key"`
				Size         int64     `xml:"Size"`
				LastModified time.Time `xml:"LastModified"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read listing of %s: %w", prefix, err)
		}

		for _, content := range page.Contents {
			objects = append(objects, Object{Key: content.Key, Size: content.Size, LastModified: content.LastModified})
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			return objects, nil
		}
		token = page.NextContinuationToken
	}
}

// newRequest builds a request for key in the bucket, or for the bucket itself when key is "".
func (b *Bucket) newRequest(ctx context.Context, method, key string, query url.Values, body io.Reader) (*http.Request, error) {
	target := *b.endpoint
	target.Path = "/" + b.bucket
	if key != "" {
		target.Path += "/" + key
	}
	target.RawPath = escapePath(target.Path)
	target.RawQuery = canonicalQuery(query)

	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	return request, nil
}

// send signs and sends request, and turns any status other than 2xx into an error that holds
// the start of the service's answer, never the credentials.
func (b *Bucket) send(request *http.Request, payloadHash string) (*http.Response, error) {
	b.sign(request, payloadHash, b.now().UTC())

	response, err := b.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorDetail))
		response.Body.Close()

		var failure struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		if xml.Unmarshal(detail, &failure) == nil && failure.Code != "" {
			return nil, fmt.Errorf("status %d: %s: %s", response.StatusCode, failure.Code, failure.Message)
		}
		return nil, fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}
	return response, nil
}

// sign adds AWS Signature Version 4 headers to request, signing the host and every header it
// already has.
func (b *Bucket) sign(request *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	request.Header.Set("X-Amz-Date", amzDate)
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)

	headers := map[string]string{"host": request.URL.Host}
	for name, values := range request.Header {
		headers[strings.ToLower(name)] = strings.TrimSpace(strings.Join(values, ","))
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headers[name])
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		request.Method, request.URL.EscapedPath(), request.URL.RawQuery, canonicalHeaders.String(), signedHeaders, payloadHash,
	}, "\n")
	scope := date + "/" + b.region + "/" + s3Service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])

	key := hmacSHA256([]byte("AWS4"+b.secretAccessKey), date)
	for _, part := range []string{b.region, s3Service, "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	request.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		b.accessKeyID, scope, signedHeaders, signature))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// escapePath percent-encodes a path the way Signature Version 4 expects: everything except
// unreserved characters and the slashes between segments.
func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		segments[index] = uriEncode(segment)
	}
	return strings.Join(segments, "/")
}

// canonicalQuery encodes query sorted by name, as Signature Version 4 expects.
func canonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	names := make([]string, 0, len(query))
	for name := range query {
		names = append(names, name)
	}
	sort.Strings(names)

	var parts []string
	for _, name := range names {
		values := append([]string(nil), query[name]...)
		sort.Strings(values)
		for _, value := range values {
			parts = append(parts, uriEncode(name)+"="+uriEncode(value))
		}
	}
	return strings.Join(parts, "&")
}

func uriEncode(value string) string {
	var encoded strings.Builder
	for _, b := range []byte(value) {
		if ('A' <= b && b <= 'Z') || ('a' <= b && b <= 'z') || ('0' <= b && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~' {
			encoded.WriteByte(b)
		} else {
			fmt.Fprintf(&encoded, "%%%02X", b)
		}
	}
	return encoded.String()
}
