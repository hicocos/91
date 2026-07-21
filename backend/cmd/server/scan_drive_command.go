package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/config"
	"github.com/video-site/backend/internal/drives/scriptcrawler"
	"github.com/video-site/backend/internal/fingerprint"
	"github.com/video-site/backend/internal/preview"
	"github.com/video-site/backend/internal/proxy"
)

func runScanDriveCommand(ctx context.Context, cfgPath, driveID string) error {
	driveID = strings.TrimSpace(driveID)
	if driveID == "" {
		return fmt.Errorf("usage: server scan-drive <drive-id>")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DBPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Storage.LocalPreviewDir, 0o755); err != nil {
		return err
	}
	cat, err := catalog.Open(cfg.Storage.DBPath)
	if err != nil {
		return fmt.Errorf("open catalog: %w", err)
	}
	defer cat.Close()
	if _, err := cat.GetDrive(ctx, driveID); err != nil {
		return fmt.Errorf("get drive %q: %w", driveID, err)
	}
	app := &App{
		cfg: cfg, cat: cat, registry: proxy.NewRegistry(),
		workers: make(map[string]*preview.Worker), thumbWorkers: make(map[string]*preview.ThumbWorker),
		fingerprintWorkers: make(map[string]*fingerprint.Worker), scriptCrawlers: make(map[string]*scriptcrawler.Crawler),
	}
	app.proxy = proxy.New(app.registry)
	app.runScan(ctx, driveID)
	return nil
}
