package pikpak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/video-site/backend/internal/drives"
)

const (
	filesURL       = "https://api-drive.mypikpak.net/drive/v1/files"
	signinURL      = "https://user.mypikpak.net/v1/auth/signin"
	tokenURL       = "https://user.mypikpak.net/v1/auth/token"
	captchaInitURL = "https://user.mypikpak.net/v1/shield/captcha/init"
)

type Driver struct {
	id               string
	rootID           string
	username         string
	password         string
	platform         string
	refreshToken     string
	accessToken      string
	captchaToken     string
	deviceID         string
	userID           string
	disableMediaLink bool

	clientID      string
	clientSecret  string
	clientVersion string
	packageName   string
	algorithms    []string
	userAgent     string

	client        *resty.Client
	onTokenUpdate func(access, refresh, captcha, deviceID string)
}

type Config struct {
	ID               string
	Username         string
	Password         string
	Platform         string
	RefreshToken     string
	AccessToken      string
	CaptchaToken     string
	DeviceID         string
	RootID           string
	DisableMediaLink bool
	OnTokenUpdate    func(access, refresh, captcha, deviceID string)
}

func New(c Config) *Driver {
	rootID := strings.TrimSpace(c.RootID)
	if rootID == "0" {
		rootID = ""
	}
	platform := strings.ToLower(strings.TrimSpace(c.Platform))
	if platform == "" {
		platform = "web"
	}
	deviceID := strings.TrimSpace(c.DeviceID)
	if deviceID == "" {
		seed := c.Username + c.Password
		if seed == "" {
			seed = c.ID
		}
		deviceID = md5Hex(seed)
	}
	d := &Driver{
		id:               c.ID,
		rootID:           rootID,
		username:         c.Username,
		password:         c.Password,
		platform:         platform,
		refreshToken:     c.RefreshToken,
		accessToken:      c.AccessToken,
		captchaToken:     c.CaptchaToken,
		deviceID:         deviceID,
		disableMediaLink: c.DisableMediaLink,
		onTokenUpdate:    c.OnTokenUpdate,
		client: resty.New().
			SetTimeout(30*time.Second).
			SetHeader("Accept", "application/json, text/plain, */*"),
	}
	d.applyPlatformDefaults()
	return d
}

func (d *Driver) Kind() string   { return "pikpak" }
func (d *Driver) ID() string     { return d.id }
func (d *Driver) RootID() string { return d.rootID }

func (d *Driver) Init(ctx context.Context) error {
	if d.refreshToken != "" {
		if err := d.refresh(ctx, d.refreshToken); err != nil {
			return err
		}
	} else {
		if err := d.login(ctx); err != nil {
			return err
		}
	}
	if err := d.refreshCaptchaTokenAtLogin(ctx, getAction(http.MethodGet, filesURL), d.userID); err != nil {
		return err
	}
	d.persistTokens()
	return nil
}

func (d *Driver) List(ctx context.Context, dirID string) ([]drives.Entry, error) {
	if dirID == "" {
		dirID = d.rootID
	}
	files, err := d.getFiles(ctx, dirID)
	if err != nil {
		return nil, err
	}
	out := make([]drives.Entry, 0, len(files))
	for _, f := range files {
		out = append(out, fileToEntry(f, dirID))
	}
	return out, nil
}

func (d *Driver) Stat(ctx context.Context, fileID string) (*drives.Entry, error) {
	var f file
	err := d.request(ctx, filesURL+"/"+fileID, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"_magic":         "2021",
			"usage":          "FETCH",
			"thumbnail_size": "SIZE_LARGE",
		})
	}, &f)
	if err != nil {
		return nil, fmt.Errorf("pikpak stat: %w", err)
	}
	e := fileToEntry(f, "")
	return &e, nil
}

func (d *Driver) StreamURL(ctx context.Context, fileID string) (*drives.StreamLink, error) {
	var f file
	usage := "FETCH"
	if !d.disableMediaLink {
		usage = "CACHE"
	}
	err := d.request(ctx, filesURL+"/"+fileID, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"_magic":         "2021",
			"usage":          usage,
			"thumbnail_size": "SIZE_LARGE",
		})
	}, &f)
	if err != nil {
		return nil, fmt.Errorf("pikpak download url: %w", err)
	}

	url := f.WebContentLink
	expires := time.Now().Add(10 * time.Minute)
	if !d.disableMediaLink {
		if m, ok := pickMediaLink(f.Medias); ok {
			url = m.Link.URL
			if !m.Link.Expire.IsZero() {
				expires = m.Link.Expire
			}
		}
	}
	if url == "" {
		return nil, errors.New("pikpak download url: empty")
	}
	headers := http.Header{}
	if d.userAgent != "" {
		headers.Set("User-Agent", d.userAgent)
	}
	return &drives.StreamLink{
		URL:     url,
		Headers: headers,
		Expires: expires,
	}, nil
}

// Upload 的真正实现见 upload.go。

// Rename 把 fileID 这个文件改名为 newName（不能是空字符串）。
// PikPak API：PATCH /drive/v1/files/<id> 带 body {"name": newName}。
// 与 OpenList drivers/pikpak/driver.go 的 Rename 行为一致。
func (d *Driver) Rename(ctx context.Context, fileID, newName string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return errors.New("pikpak rename: empty file id")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("pikpak rename: empty new name")
	}
	if err := d.request(ctx, filesURL+"/"+fileID, http.MethodPatch, func(req *resty.Request) {
		req.SetBody(map[string]any{"name": newName})
	}, nil); err != nil {
		return fmt.Errorf("pikpak rename: %w", err)
	}
	return nil
}

func (d *Driver) EnsureDir(ctx context.Context, pathFromRoot string) (string, error) {
	return "", drives.ErrNotSupported
}

func (d *Driver) getFiles(ctx context.Context, parentID string) ([]file, error) {
	out := make([]file, 0)
	pageToken := "first"
	for pageToken != "" {
		if pageToken == "first" {
			pageToken = ""
		}
		query := map[string]string{
			"parent_id":      parentID,
			"thumbnail_size": "SIZE_LARGE",
			"with_audit":     "true",
			"limit":          "100",
			"filters":        `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
			"page_token":     pageToken,
		}
		var resp filesResp
		if err := d.request(ctx, filesURL, http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(query)
		}, &resp); err != nil {
			return nil, fmt.Errorf("pikpak list: %w", err)
		}
		out = append(out, resp.Files...)
		pageToken = resp.NextPageToken
	}
	return out, nil
}

func (d *Driver) request(ctx context.Context, url, method string, configure func(*resty.Request), out any) error {
	return d.requestOnce(ctx, url, method, configure, out, true)
}

func (d *Driver) requestOnce(ctx context.Context, url, method string, configure func(*resty.Request), out any, retry bool) error {
	req := d.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", d.userAgent).
		SetHeader("X-Device-ID", d.deviceID).
		SetHeader("X-Captcha-Token", d.captchaToken)
	if d.accessToken != "" {
		req.SetHeader("Authorization", "Bearer "+d.accessToken)
	}
	if configure != nil {
		configure(req)
	}
	if out != nil {
		req.SetResult(out)
	}
	var e errResp
	req.SetError(&e)

	res, err := req.Execute(method, url)
	if err != nil {
		return err
	}
	if e.isError() {
		switch e.ErrorCode {
		case 4122, 4121, 16:
			if retry {
				if err := d.refresh(ctx, d.refreshToken); err != nil {
					return err
				}
				return d.requestOnce(ctx, url, method, configure, out, false)
			}
		case 9:
			if retry {
				if err := d.refreshCaptchaTokenAtLogin(ctx, getAction(method, url), d.userID); err != nil {
					return err
				}
				return d.requestOnce(ctx, url, method, configure, out, false)
			}
		}
		return &e
	}
	if res.IsError() {
		return fmt.Errorf("pikpak http %d: %s", res.StatusCode(), string(res.Body()))
	}
	return nil
}

func pickMediaLink(items []media) (media, bool) {
	if len(items) == 0 {
		return media{}, false
	}
	for _, m := range items {
		if m.IsOrigin && m.Link.URL != "" {
			return m, true
		}
	}
	for _, m := range items {
		if m.IsDefault && m.Link.URL != "" {
			return m, true
		}
	}
	for _, m := range items {
		if m.Link.URL != "" {
			return m, true
		}
	}
	return media{}, false
}

func guessMime(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/x-msvideo"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	}
	return "application/octet-stream"
}

func ParseBoolDefault(raw string, def bool) bool {
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return v
}

var _ drives.Drive = (*Driver)(nil)
