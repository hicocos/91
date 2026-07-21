package storageproviders

import (
	"strings"
	"testing"
)

func TestGoogleDriveManifestUsesChineseSharedDriveGuidance(t *testing.T) {
	descriptor, ok := DefaultRegistry().Lookup("googledrive")
	if !ok {
		t.Fatal("missing googledrive manifest")
	}
	for _, field := range descriptor.Manifest.Fields {
		if field.Key != "shared_drive_id" {
			continue
		}
		if field.Label != "共享云端硬盘（团队盘）ID" || !strings.Contains(field.Help, "无需重复填写") {
			t.Fatalf("field = %+v", field)
		}
		return
	}
	t.Fatal("missing shared_drive_id field")
}
