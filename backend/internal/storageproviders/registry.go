package storageproviders

import (
	"errors"
	"sort"
	"strings"
)

type FieldManifest struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required,omitempty"`
	Sensitive    bool   `json:"sensitive,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Help         string `json:"help,omitempty"`
}
type Capabilities struct {
	List   bool `json:"list"`
	Play   bool `json:"play"`
	Upload bool `json:"upload"`
	Delete bool `json:"delete"`
}
type ProviderManifest struct {
	Kind         string          `json:"kind"`
	DisplayName  string          `json:"displayName"`
	AuthMethods  []string        `json:"authMethods"`
	RootMode     string          `json:"rootMode"`
	DefaultRoot  string          `json:"defaultRoot"`
	Fields       []FieldManifest `json:"fields"`
	Capabilities Capabilities    `json:"capabilities"`
	Legacy       bool            `json:"legacy,omitempty"`
}
type Descriptor struct{ Manifest ProviderManifest }
type Registry struct{ descriptors map[string]Descriptor }

func NewRegistry() *Registry { return &Registry{descriptors: map[string]Descriptor{}} }
func (r *Registry) Register(d Descriptor) error {
	k := strings.TrimSpace(d.Manifest.Kind)
	if k == "" {
		return errors.New("provider kind required")
	}
	if _, ok := r.descriptors[k]; ok {
		return errors.New("duplicate provider: " + k)
	}
	d.Manifest.Kind = k
	r.descriptors[k] = d
	return nil
}
func (r *Registry) Lookup(kind string) (Descriptor, bool) { d, ok := r.descriptors[kind]; return d, ok }
func (r *Registry) Manifests() []ProviderManifest {
	out := make([]ProviderManifest, 0, len(r.descriptors))
	for _, d := range r.descriptors {
		out = append(out, d.Manifest)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	rw := Capabilities{List: true, Play: true, Upload: true, Delete: true}
	readDelete := Capabilities{List: true, Play: true, Delete: true}
	target := []ProviderManifest{
		{Kind: "onedrive", DisplayName: "OneDrive", AuthMethods: []string{"oauth", "manual"}, RootMode: "item-id", DefaultRoot: "root", Capabilities: rw, Fields: []FieldManifest{{Key: "client_id", Label: "Client ID", Required: true, Help: "Microsoft Entra 应用程序（客户端）ID"}, {Key: "client_secret", Label: "Client Secret", Sensitive: true, Help: "公共客户端可留空；机密客户端填写密码的 Value"}, {Key: "tenant", Label: "Tenant", DefaultValue: "common", Help: "通常保持 common"}, {Key: "refresh_token", Label: "Refresh Token", Sensitive: true, Help: "授权连接会自动填入"}, {Key: "site_id", Label: "SharePoint Site ID"}, {Key: "drive_id", Label: "SharePoint Drive ID"}}},
		{Kind: "googledrive", DisplayName: "Google Drive", AuthMethods: []string{"oauth", "manual"}, RootMode: "item-id", DefaultRoot: "root", Capabilities: rw, Fields: []FieldManifest{{Key: "client_id", Label: "客户端 ID（Client ID）", Required: true, Help: "Google Cloud Web 应用 OAuth 客户端 ID"}, {Key: "client_secret", Label: "客户端密钥（Client Secret）", Sensitive: true, Help: "Google Cloud OAuth 客户端密钥"}, {Key: "refresh_token", Label: "授权令牌（Refresh Token）", Sensitive: true, Help: "点击授权连接后自动填入，通常不需要手工填写"}, {Key: "shared_drive_id", Label: "共享云端硬盘（团队盘）ID", Help: "仅团队盘需要。填写团队盘链接中 folders/ 后的 ID；扫描整个团队盘时，下方扫描起点留空，无需重复填写"}}},
		{Kind: "webdav", DisplayName: "WebDAV", AuthMethods: []string{"basic", "anonymous"}, RootMode: "path", DefaultRoot: "/", Capabilities: rw, Fields: []FieldManifest{{Key: "base_url", Label: "WebDAV 地址", Required: true, Help: "例如 https://dav.example.com/dav/"}, {Key: "username", Label: "用户名"}, {Key: "password", Label: "密码 / 应用口令", Sensitive: true}}},
		{Kind: "s3", DisplayName: "S3 兼容存储", AuthMethods: []string{"access-key"}, RootMode: "prefix", DefaultRoot: "", Capabilities: readDelete, Fields: []FieldManifest{{Key: "endpoint", Label: "Endpoint", Help: "AWS S3 可自动生成；R2、MinIO 等填写服务地址"}, {Key: "region", Label: "Region", Required: true, DefaultValue: "us-east-1"}, {Key: "bucket", Label: "Bucket", Required: true}, {Key: "access_key_id", Label: "Access Key ID", Required: true}, {Key: "secret_access_key", Label: "Secret Access Key", Required: true, Sensitive: true}, {Key: "session_token", Label: "Session Token", Sensitive: true, Help: "仅临时凭据需要"}, {Key: "force_path_style", Label: "Force path style", Type: "boolean"}}},
	}
	for _, m := range target {
		_ = r.Register(Descriptor{Manifest: m})
	}
	for _, k := range []string{"quark", "p115", "p123", "pikpak", "wopan", "guangyapan", "localstorage"} {
		_ = r.Register(Descriptor{Manifest: ProviderManifest{Kind: k, DisplayName: k, Legacy: true, Capabilities: rw}})
	}
	return r
}
