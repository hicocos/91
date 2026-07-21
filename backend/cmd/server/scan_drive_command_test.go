package main

import (
	"context"
	"strings"
	"testing"
)

func TestScanDriveCommandRejectsMissingDriveID(t *testing.T) {
	err := runScanDriveCommand(context.Background(), "unused.yaml", "")
	if err == nil || !strings.Contains(err.Error(), "usage: server scan-drive") {
		t.Fatalf("error = %v", err)
	}
}
