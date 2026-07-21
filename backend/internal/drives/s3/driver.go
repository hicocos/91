package s3

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
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/video-site/backend/internal/drives"
	"github.com/video-site/backend/internal/storageproviders"
)

const Kind = "s3"

type Config struct {
	ID, Endpoint, Region, Bucket, AccessKey, SecretKey, SessionToken, RootPrefix string
	ForcePathStyle                                                               bool
	HTTPClient                                                                   *http.Client
}
type Driver struct {
	id, endpoint, region, bucket, accessKey, secretKey, sessionToken, root string
	pathStyle                                                              bool
	client                                                                 *http.Client
	allowPrivate, allowInsecure                                            bool
}

func New(c Config) *Driver {
	ep := strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	if ep == "" {
		ep = "https://s3." + defaultValue(c.Region, "us-east-1") + ".amazonaws.com"
	}
	cl := c.HTTPClient
	if cl == nil {
		cl = storageproviders.NewEndpointHTTPClient(30 * time.Minute)
	}
	return &Driver{id: c.ID, endpoint: ep, region: defaultValue(c.Region, "us-east-1"), bucket: strings.TrimSpace(c.Bucket), accessKey: strings.TrimSpace(c.AccessKey), secretKey: c.SecretKey, sessionToken: strings.TrimSpace(c.SessionToken), root: normalizePrefix(c.RootPrefix), pathStyle: c.ForcePathStyle, client: cl}
}
func defaultValue(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}
func normalizePrefix(v string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(v), func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/") + "/"
}
func (d *Driver) Kind() string   { return Kind }
func (d *Driver) ID() string     { return d.id }
func (d *Driver) RootID() string { return d.root }
func (d *Driver) Init(ctx context.Context) error {
	if d.bucket == "" || d.accessKey == "" || d.secretKey == "" {
		return errors.New("s3 init: bucket and access keys are required")
	}
	if err := storageproviders.ValidateEndpoint(d.endpoint, nil); err != nil {
		return fmt.Errorf("s3 endpoint policy: %w", err)
	}
	_, err := d.List(ctx, d.root)
	return err
}

type listResult struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	Contents              []object       `xml:"Contents"`
	CommonPrefixes        []commonPrefix `xml:"CommonPrefixes"`
	NextContinuationToken string         `xml:"NextContinuationToken"`
	IsTruncated           bool           `xml:"IsTruncated"`
}
type object struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	ETag         string `xml:"ETag"`
	LastModified string `xml:"LastModified"`
}
type commonPrefix struct {
	Prefix string `xml:"Prefix"`
}

func (d *Driver) List(ctx context.Context, dirID string) ([]drives.Entry, error) {
	prefix, err := d.resolveDir(dirID)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return nil, errors.New("s3: bucket root listing is disabled; configure root_prefix")
	}
	token := ""
	var out []drives.Entry
	for {
		q := url.Values{"list-type": {"2"}, "delimiter": {"/"}, "prefix": {prefix}, "max-keys": {"1000"}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL(""), nil)
		req.URL.RawQuery = q.Encode()
		if err := d.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC()); err != nil {
			return nil, err
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}
		if err = check(resp); err != nil {
			return nil, err
		}
		var lr listResult
		err = xml.NewDecoder(resp.Body).Decode(&lr)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, p := range lr.CommonPrefixes {
			out = append(out, drives.Entry{ID: p.Prefix, Name: path.Base(strings.TrimSuffix(p.Prefix, "/")), IsDir: true, ParentID: prefix})
		}
		for _, o := range lr.Contents {
			if o.Key == prefix {
				continue
			}
			mt, _ := time.Parse(time.RFC3339, o.LastModified)
			out = append(out, drives.Entry{ID: o.Key, Name: path.Base(o.Key), Size: o.Size, ParentID: prefix, ModTime: mt})
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			break
		}
		token = lr.NextContinuationToken
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (d *Driver) resolveDir(v string) (string, error) {
	v = strings.TrimLeft(strings.TrimSpace(v), "/")
	if v == "" {
		if d.root == "" {
			return "", errors.New("s3: root_prefix is required")
		}
		return d.root, nil
	}
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	if d.root != "" && v != d.root && !strings.HasPrefix(v, d.root) {
		return "", errors.New("s3: directory outside root prefix")
	}
	return v, nil
}
func (d *Driver) resolveObject(v string) (string, error) {
	v = strings.TrimLeft(strings.TrimSpace(v), "/")
	if v == "" || strings.HasSuffix(v, "/") {
		return "", errors.New("s3: invalid object key")
	}
	if d.root != "" && !strings.HasPrefix(v, d.root) {
		return "", errors.New("s3: object outside root prefix")
	}
	return v, nil
}
func (d *Driver) Stat(ctx context.Context, fileID string) (*drives.Entry, error) {
	key, err := d.resolveObject(fileID)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, d.baseURL(key), nil)
	d.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err = check(resp); err != nil {
		return nil, err
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return &drives.Entry{ID: key, Name: path.Base(key), Size: size, MimeType: resp.Header.Get("Content-Type"), ParentID: path.Dir(key) + "/"}, nil
}

func (d *Driver) Remove(ctx context.Context, fileID string) error {
	key, err := d.resolveObject(fileID)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, d.baseURL(key), nil)
	d.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return check(resp)
}
func (d *Driver) StreamURL(ctx context.Context, fileID string) (*drives.StreamLink, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := d.resolveObject(fileID)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(15 * time.Minute)
	u, err := d.presign(d.baseURL(key), 15*time.Minute, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return &drives.StreamLink{URL: u, Headers: http.Header{}, Expires: expires}, nil
}
func (d *Driver) baseURL(key string) string {
	u, _ := url.Parse(d.endpoint)
	if d.pathStyle || strings.Contains(u.Hostname(), "localhost") || netIP(u.Hostname()) {
		u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(d.bucket)
	} else {
		u.Host = d.bucket + "." + u.Host
	}
	if key != "" {
		for _, s := range strings.Split(key, "/") {
			u.Path += "/" + url.PathEscape(s)
		}
	}
	return u.String()
}
func netIP(h string) bool {
	return strings.Count(h, ".") == 3 && strings.IndexFunc(h, func(r rune) bool { return (r < '0' || r > '9') && r != '.' }) < 0
}
func check(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("s3 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}
func hmacSHA(key []byte, s string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s))
	return h.Sum(nil)
}
func (d *Driver) sign(req *http.Request, payload string, now time.Time) error {
	req.Header.Set("x-amz-content-sha256", payload)
	req.Header.Set("x-amz-date", now.Format("20060102T150405Z"))
	if d.sessionToken != "" {
		req.Header.Set("x-amz-security-token", d.sessionToken)
	}
	signed := "host;x-amz-content-sha256;x-amz-date"
	headers := "host:" + req.URL.Host + "\n" + "x-amz-content-sha256:" + payload + "\n" + "x-amz-date:" + now.Format("20060102T150405Z") + "\n"
	if d.sessionToken != "" {
		signed += ";x-amz-security-token"
		headers += "x-amz-security-token:" + d.sessionToken + "\n"
	}
	canon := req.Method + "\n" + req.URL.EscapedPath() + "\n" + req.URL.Query().Encode() + "\n" + headers + "\n" + signed + "\n" + payload
	scope := now.Format("20060102") + "/" + d.region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canon))
	sts := "AWS4-HMAC-SHA256\n" + now.Format("20060102T150405Z") + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	k := hmacSHA([]byte("AWS4"+d.secretKey), now.Format("20060102"))
	k = hmacSHA(k, d.region)
	k = hmacSHA(k, "s3")
	k = hmacSHA(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA(k, sts))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+d.accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
	return nil
}
func (d *Driver) presign(raw string, ttl time.Duration, now time.Time) (string, error) {
	u, _ := url.Parse(raw)
	q := u.Query()
	scope := now.Format("20060102") + "/" + d.region + "/s3/aws4_request"
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", d.accessKey+"/"+scope)
	q.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	q.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	if d.sessionToken != "" {
		q.Set("X-Amz-Security-Token", d.sessionToken)
	}
	u.RawQuery = q.Encode()
	canon := "GET\n" + u.EscapedPath() + "\n" + u.RawQuery + "\nhost:" + u.Host + "\n\nhost\nUNSIGNED-PAYLOAD"
	sum := sha256.Sum256([]byte(canon))
	sts := "AWS4-HMAC-SHA256\n" + now.Format("20060102T150405Z") + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	k := hmacSHA([]byte("AWS4"+d.secretKey), now.Format("20060102"))
	k = hmacSHA(k, d.region)
	k = hmacSHA(k, "s3")
	k = hmacSHA(k, "aws4_request")
	q.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA(k, sts)))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
