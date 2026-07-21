package storageproviders

import "testing"

func TestDefaultRegistryTargetManifests(t *testing.T) {
	r := DefaultRegistry()
	for _, kind := range []string{"onedrive", "googledrive", "webdav", "s3"} {
		d, ok := r.Lookup(kind)
		if !ok {
			t.Fatalf("missing descriptor %q", kind)
		}
		if d.Manifest.Kind != kind || d.Manifest.DisplayName == "" || len(d.Manifest.Fields) == 0 {
			t.Fatalf("bad manifest for %s: %+v", kind, d.Manifest)
		}
	}
	s3, _ := r.Lookup("s3")
	if !s3.Manifest.Capabilities.List || !s3.Manifest.Capabilities.Play || s3.Manifest.Capabilities.Upload || !s3.Manifest.Capabilities.Delete {
		t.Fatalf("s3 capabilities = %+v; want read/play/delete without upload", s3.Manifest.Capabilities)
	}
	if !hasField(s3.Manifest.Fields, "bucket") || !hasField(s3.Manifest.Fields, "secret_access_key") {
		t.Fatal("s3 manifest missing required fields")
	}
	od, _ := r.Lookup("onedrive")
	if !contains(od.Manifest.AuthMethods, "oauth") || !contains(od.Manifest.AuthMethods, "manual") {
		t.Fatalf("onedrive auth methods = %v", od.Manifest.AuthMethods)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	d := Descriptor{Manifest: ProviderManifest{Kind: "x", DisplayName: "X"}}
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(d); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func hasField(fields []FieldManifest, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
