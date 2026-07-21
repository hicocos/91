package main

import (
	"strings"

	"github.com/video-site/backend/internal/scanner"
)

func (a *App) updateDriveMediaScanProgress(driveID string, stats scanner.Stats) {
	driveID = strings.TrimSpace(driveID)
	if driveID == "" {
		return
	}
	a.scanQueueMu.Lock()
	defer a.scanQueueMu.Unlock()
	if !a.scanQueued[driveID] {
		return
	}
	if a.scanProgress == nil {
		a.scanProgress = make(map[string]driveScanProgress)
	}
	progress := a.scanProgress[driveID]
	progress.Scanned, progress.Added = stats.Scanned, stats.Added
	progress.VideoScanned, progress.AudioScanned = stats.VideoScanned, stats.AudioScanned
	progress.VideoAdded, progress.AudioAdded = stats.VideoAdded, stats.AudioAdded
	a.scanProgress[driveID] = progress
}
