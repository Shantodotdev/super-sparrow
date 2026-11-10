package r2

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSignMatchesTheAWSExample checks the signer against the GET Object example in the AWS
// Signature Version 4 documentation for Amazon S3.
func TestSignMatchesTheAWSExample(t *testing.T) {
	endpoint, _ := url.Parse("https://examplebucket.s3.amazonaws.com")
	bucket := newBucket(endpoint, "us-east-1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", time.Now)

	request, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	request.Header.Set("Range", "bytes=0-9")
	bucket.sign(request, emptyHash, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := request.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

func TestCanonicalQueryAndPathEncoding(t *testing.T) {
	if got := canonicalQuery(url.Values{"prefix": {"daily/"}, "list-type": {"2"}}); got != "list-type=2&prefix=daily%2F" {
		t.Errorf("canonicalQuery = %q", got)
	}
	if got := escapePath("/bucket/daily/sparrow 1.db"); !strings.HasSuffix(got, "/daily/sparrow%201.db") {
		t.Errorf("escapePath = %q", got)
	}
}

func TestPresignGetMatchesTheAWSExample(t *testing.T) {
	endpoint, _ := url.Parse("https://examplebucket.s3.amazonaws.com")
	bucket := newBucket(endpoint, "us-east-1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", time.Now)
	bucket.now = func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) }

	// The documentation's example presigns /test.txt for 86400 seconds, with no extra query.
	link := bucket.presign(http.MethodGet, "test.txt", url.Values{}, nil, 86400*time.Second)
	want := "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if !strings.HasSuffix(link, want) {
		t.Fatalf("presigned link %s does not end with %s", link, want)
	}
}

func TestPresignPutSignsTheContentType(t *testing.T) {
	endpoint, _ := url.Parse("https://acct.r2.cloudflarestorage.com")
	bucket := newBucket(endpoint, "auto", "key", "secret", "files", time.Now)

	link := bucket.PresignPut("abc", "video/mp4", time.Hour)
	if !strings.Contains(link, "X-Amz-SignedHeaders=content-type%3Bhost") || !strings.HasPrefix(link, "https://acct.r2.cloudflarestorage.com/files/abc?") {
		t.Fatalf("unexpected link %s", link)
	}
}
