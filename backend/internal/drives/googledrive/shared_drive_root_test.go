package googledrive

import "testing"

func TestSharedDriveUsesDriveIDAsDefaultRoot(t *testing.T) {
	d := New(Config{ID: "g", RootID: "root", SharedDriveID: "team-drive-id"})
	if d.RootID() != "team-drive-id" {
		t.Fatalf("root = %q, want shared drive id", d.RootID())
	}
}

func TestSharedDrivePreservesExplicitFolderRoot(t *testing.T) {
	d := New(Config{ID: "g", RootID: "folder-id", SharedDriveID: "team-drive-id"})
	if d.RootID() != "folder-id" {
		t.Fatalf("root = %q, want explicit folder id", d.RootID())
	}
}
