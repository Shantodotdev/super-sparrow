package r2

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
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
	r2Region  = "auto"
	s3Service = "s3"
	r2Timeout = 10 * time.Minute
	// A request that is not an upload or download of a file must finish in this time.
	r2ListTimeout  = 5 * time.Minute
	emptyHash      = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	maxErrorDetail = 4 << 10
)

// ErrNotFound means the bucket has no object under the requested key.
var ErrNotFound = errors.New("object not found")

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
	// A file can be several gigabytes, so the client has no overall timeout: the caller's context
	// ends a request. The transport still gives up on a server that accepts a request, or the end
	// of an upload, and then stays silent.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = r2Timeout

	return &Bucket{
		endpoint: endpoint, bucket: bucket, region: region, accessKeyID: accessKeyID,
		secretAccessKey: secretAccessKey, http: &http.Client{Transport: transport}, now: now,
	}
}

// Put uploads the file at path as key in a single request, which R2 accepts up to 5 GiB. The
// file is read twice, once to hash it for the signature and once to send it, and is never held in
// memory. The bucket replaces an existing object only once the new upload has completed, so a
// failed upload leaves the old copy in place.
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
	ctx, cancel := context.WithTimeout(ctx, r2ListTimeout)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(ctx, r2ListTimeout)
	defer cancel()

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

		var notFound error
		if response.StatusCode == http.StatusNotFound {
			notFound = ErrNotFound
		}

		var failure struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		if xml.Unmarshal(detail, &failure) == nil && failure.Code != "" {
			return nil, errors.Join(notFound, fmt.Errorf("status %d: %s: %s", response.StatusCode, failure.Code, failure.Message))
		}
		return nil, errors.Join(notFound, fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(detail))))
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

// NewBucketAt returns a bucket reached at endpoint, such as a test server standing in for R2.
func NewBucketAt(endpoint *url.URL, region, accessKeyID, secretAccessKey, bucket string) *Bucket {
	return newBucket(endpoint, region, accessKeyID, secretAccessKey, bucket, time.Now)
}

// Head returns the size of key's object, or ErrNotFound when the bucket has none.
func (b *Bucket) Head(ctx context.Context, key string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, r2ListTimeout)
	defer cancel()

	request, err := b.newRequest(ctx, http.MethodHead, key, nil, nil)
	if err != nil {
		return 0, err
	}
	response, err := b.send(request, emptyHash)
	if err != nil {
		return 0, fmt.Errorf("inspect %s: %w", key, err)
	}
	response.Body.Close()
	return response.ContentLength, nil
}

// GetPrefix returns the first limit bytes of key's object, or all of it when it is shorter.
func (b *Bucket) GetPrefix(ctx context.Context, key string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r2ListTimeout)
	defer cancel()

	request, err := b.newRequest(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=0-%d", limit-1))

	response, err := b.send(request, emptyHash)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return data, nil
}

// PresignPut returns a link that lets whoever holds it upload key's object with a PUT that
// carries exactly the given Content-Type, until ttl has passed.
func (b *Bucket) PresignPut(key, contentType string, ttl time.Duration) string {
	return b.presign(http.MethodPut, key, url.Values{}, map[string]string{"content-type": contentType}, ttl)
}

// PresignGet returns a link that lets whoever holds it download key's object until ttl has
// passed. The bucket answers with the given content type and Content-Disposition instead of
// the ones stored with the object.
func (b *Bucket) PresignGet(key, contentType, disposition string, ttl time.Duration) string {
	query := url.Values{"response-content-type": {contentType}, "response-content-disposition": {disposition}}
	return b.presign(http.MethodGet, key, query, nil, ttl)
}

// presign signs a request into its query string (AWS Signature Version 4), signing the host and
// the given headers, which the caller must then send unchanged.
func (b *Bucket) presign(method, key string, query url.Values, headers map[string]string, ttl time.Duration) string {
	now := b.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	scope := date + "/" + b.region + "/" + s3Service + "/aws4_request"

	target := *b.endpoint
	target.Path = "/" + key
	if b.bucket != "" {
		target.Path = "/" + b.bucket + target.Path
	}
	target.RawPath = escapePath(target.Path)

	signed := map[string]string{"host": target.Host}
	for name, value := range headers {
		signed[strings.ToLower(name)] = strings.TrimSpace(value)
	}
	names := make([]string, 0, len(signed))
	for name := range signed {
		names = append(names, name)
	}
	sort.Strings(names)
	signedHeaders := strings.Join(names, ";")

	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", b.accessKeyID+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprint(int64(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	canonical := canonicalQuery(query)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + signed[name] + "\n")
	}
	canonicalRequest := strings.Join([]string{
		method, target.EscapedPath(), canonical, canonicalHeaders.String(), signedHeaders, "UNSIGNED-PAYLOAD",
	}, "\n")
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])

	signingKey := hmacSHA256([]byte("AWS4"+b.secretAccessKey), date)
	for _, part := range []string{b.region, s3Service, "aws4_request"} {
		signingKey = hmacSHA256(signingKey, part)
	}

	target.RawQuery = canonical + "&X-Amz-Signature=" + hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	return target.String()
}

// Origin returns the address browsers reach the bucket at, for the page's content security
// policy and the bucket's CORS rule.
func (b *Bucket) Origin() string {
	return b.endpoint.Scheme + "://" + b.endpoint.Host
}
