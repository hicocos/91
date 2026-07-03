package main

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/video-site/backend/internal/api"
	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives/scriptcrawler"
	"github.com/video-site/backend/internal/mediasim"
)

const (
	tagRetagUpgradeMarker        = "tags.retag.v2_done"
	tagMaintenanceLastRunSetting = "tags.maintenance.last_run_ms"
	tagRetagBatchSize            = 500
	tagClusterTitleThreshold     = 0.85
)

func (a *App) beginTagJob(kind string) bool {
	if a == nil || a.cat == nil {
		return false
	}
	a.tagJobMu.Lock()
	defer a.tagJobMu.Unlock()
	if a.tagJobState.Running {
		return false
	}
	a.tagJobState = api.TagJobStatus{
		State:     "running",
		Running:   true,
		Kind:      kind,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	return true
}

func (a *App) finishTagJob(state string, err error) {
	a.tagJobMu.Lock()
	defer a.tagJobMu.Unlock()
	a.tagJobState.State = state
	a.tagJobState.Running = false
	a.tagJobState.LastFinishedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		a.tagJobState.LastError = err.Error()
	}
}

func (a *App) tagJobStatus() api.TagJobStatus {
	if a == nil {
		return api.TagJobStatus{State: "idle"}
	}
	a.tagJobMu.Lock()
	status := a.tagJobState
	a.tagJobMu.Unlock()
	if status.State == "" {
		status.State = "idle"
	}
	return status
}

func (a *App) startTagRetag(ctx context.Context) bool {
	return a.startTagRetagInternal(ctx, false)
}

func (a *App) startTagRetagInternal(ctx context.Context, writeUpgradeMarker bool) bool {
	if !a.beginTagJob("retag") {
		return false
	}
	go a.runTagRetag(ctx, writeUpgradeMarker)
	return true
}

func (a *App) startPostStartupTagMaintenance(ctx context.Context) {
	if a == nil || a.cat == nil {
		return
	}
	marker, err := a.cat.GetSetting(ctx, tagRetagUpgradeMarker, "")
	if err != nil {
		log.Printf("[tag-retag] post-startup maintenance skipped because upgrade marker cannot be read: %v", err)
		return
	}
	if marker == "1" {
		return
	}
	if !a.beginTagJob("retag") {
		log.Printf("[tag-retag] post-startup maintenance not started because another tag job is running")
		return
	}
	go a.runPostStartupTagMaintenance(ctx)
}

func (a *App) runPostStartupTagMaintenance(ctx context.Context) {
	a.tagMaintenanceMu.Lock()
	defer a.tagMaintenanceMu.Unlock()

	marker, err := a.cat.GetSetting(ctx, tagRetagUpgradeMarker, "")
	if err != nil {
		a.finishTagJob("failed", err)
		return
	}
	total, err := a.cat.CountVideosForRetag(ctx, 0)
	if err != nil {
		a.finishTagJob("failed", err)
		return
	}
	a.tagJobMu.Lock()
	a.tagJobState.Total = total
	a.tagJobMu.Unlock()

	log.Printf("[tag-retag] post-startup tag maintenance started")
	if err := a.cat.RunPostStartupTagMaintenance(ctx); err != nil {
		a.finishTagJob("failed", err)
		log.Printf("[tag-retag] post-startup tag maintenance failed: %v", err)
		return
	}
	if marker != "1" {
		// The first upgrade run performs the authoritative auto/legacy rebuild
		// after legacy relations have been normalized.
		a.tagJobMu.Lock()
		a.tagJobState.Processed = 0
		a.tagJobMu.Unlock()
		a.runTagRetagLocked(ctx, true, false)
		return
	}
	a.tagJobMu.Lock()
	a.tagJobState.Processed = total
	a.tagJobMu.Unlock()
	a.finishTagJob("completed", nil)
	log.Printf("[tag-retag] post-startup tag maintenance completed")
}

func (a *App) runTagRetag(ctx context.Context, writeUpgradeMarker bool) {
	a.tagMaintenanceMu.Lock()
	defer a.tagMaintenanceMu.Unlock()
	a.runTagRetagLocked(ctx, writeUpgradeMarker, true)
}

func (a *App) runTagRetagLocked(ctx context.Context, writeUpgradeMarker bool, resetGeneratedState bool) {
	total, err := a.cat.CountVideosForRetag(ctx, 0)
	if err != nil {
		a.finishTagJob("failed", err)
		return
	}
	a.tagJobMu.Lock()
	a.tagJobState.Total = total
	a.tagJobMu.Unlock()

	autoGenerateEnabled := true
	if resetGeneratedState {
		enabled, err := a.cat.AutoGenerateTagsEnabled(ctx)
		if err != nil {
			a.finishTagJob("failed", err)
			return
		}
		autoGenerateEnabled = enabled
		if _, err := a.cat.ResetGeneratedTagState(ctx); err != nil {
			a.finishTagJob("failed", err)
			return
		}
		if err := a.cat.ReconcileBuiltinTags(ctx); err != nil {
			a.finishTagJob("failed", err)
			return
		}
	}

	if autoGenerateEnabled {
		matcher, err := a.cat.Matcher(ctx)
		if err != nil {
			a.finishTagJob("failed", err)
			return
		}
		lastID := ""
		for {
			if err := ctx.Err(); err != nil {
				a.finishTagJob("canceled", err)
				return
			}
			processed, nextID, done, err := a.cat.RetagVideosBatch(ctx, matcher, lastID, tagRetagBatchSize, 0)
			if err != nil {
				a.finishTagJob("failed", err)
				return
			}
			a.tagJobMu.Lock()
			a.tagJobState.Processed += processed
			a.tagJobMu.Unlock()
			lastID = nextID
			if done {
				break
			}
		}
	} else {
		a.tagJobMu.Lock()
		a.tagJobState.Processed = total
		a.tagJobMu.Unlock()
	}

	if err := a.ensureAllScriptCrawlerNameTags(ctx); err != nil {
		log.Printf("[tag-retag] ensure crawler name tags: %v", err)
	}
	if autoGenerateEnabled {
		if added, err := a.cat.SyncSeriesTags(ctx, 3); err != nil {
			a.finishTagJob("failed", err)
			return
		} else if added > 0 {
			log.Printf("[tag-retag] series tags added=%d", added)
		}
	}
	if _, err := a.cat.PruneUnreferencedTags(ctx); err != nil {
		a.finishTagJob("failed", err)
		return
	}
	if writeUpgradeMarker {
		if err := a.cat.SetSetting(ctx, tagRetagUpgradeMarker, "1"); err != nil {
			a.finishTagJob("failed", err)
			return
		}
	}
	a.finishTagJob("completed", nil)
}

func (a *App) ensureAllScriptCrawlerNameTags(ctx context.Context) error {
	drives, err := a.cat.ListDrives(ctx)
	if err != nil {
		return err
	}
	for _, drive := range drives {
		if drive == nil || drive.Kind != scriptcrawler.Kind {
			continue
		}
		tagName := strings.TrimSpace(drive.Name)
		if tagName == "" {
			tagName = strings.TrimSpace(drive.ID)
		}
		if tagName == "" {
			continue
		}
		prefix := scriptcrawler.BuildVideoID(drive.ID, "")
		if _, err := a.cat.EnsureCrawlerTagForVideoIDPrefix(ctx, prefix, tagName); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) runNightlyTagMaintenance(ctx context.Context) error {
	if a == nil || a.cat == nil {
		return nil
	}
	a.tagMaintenanceMu.Lock()
	defer a.tagMaintenanceMu.Unlock()

	var stepErrors []error
	runStep := func(name string, fn func() error) {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[tag-maintenance] %s", name)
		if err := fn(); err != nil {
			log.Printf("[tag-maintenance] %s: %v", name, err)
			stepErrors = append(stepErrors, errors.New(name+": "+err.Error()))
		}
	}

	runStep("incremental retag", func() error {
		raw, err := a.cat.GetSetting(ctx, tagMaintenanceLastRunSetting, "0")
		if err != nil {
			return err
		}
		since, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		highWaterMark := time.Now().UnixMilli()
		matcher, err := a.cat.Matcher(ctx)
		if err != nil {
			return err
		}
		lastID := ""
		for {
			_, nextID, done, err := a.cat.RetagVideosBatch(ctx, matcher, lastID, tagRetagBatchSize, since)
			if err != nil {
				return err
			}
			lastID = nextID
			if done {
				break
			}
		}
		// Persist the start-of-step high-water mark. Updates that race this pass
		// are intentionally eligible for the next nightly run.
		return a.cat.SetSetting(ctx, tagMaintenanceLastRunSetting, strconv.FormatInt(highWaterMark, 10))
	})
	runStep("series tags", func() error {
		added, err := a.cat.SyncSeriesTags(ctx, 3)
		log.Printf("[tag-maintenance] series tags added=%d", added)
		return err
	})
	runStep("clear propagated tags", func() error {
		affected, err := a.cat.ClearPropagatedTags(ctx)
		log.Printf("[tag-maintenance] propagated tags cleared_from=%d", affected)
		return err
	})
	runStep("duplicate propagation", func() error {
		added, err := a.cat.PropagateTagsAcrossDuplicates(ctx)
		log.Printf("[tag-maintenance] duplicate propagation added=%d", added)
		return err
	})
	runStep("title cluster propagation", func() error {
		added, err := a.propagateTagsAcrossTitleClusters(ctx)
		log.Printf("[tag-maintenance] title cluster propagation added=%d", added)
		return err
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.Join(stepErrors...)
}

type tagClusterCandidate struct {
	video   *catalog.Video
	keys    []string
	qgrams  map[string]struct{}
	buckets []string
}

func collectTagTitleClusters(videos []*catalog.Video) [][]*catalog.Video {
	candidates := make([]tagClusterCandidate, 0, len(videos))
	for _, video := range videos {
		if video == nil || strings.TrimSpace(video.ID) == "" || strings.TrimSpace(video.Title) == "" {
			continue
		}
		keys := mediasim.TitleKeys(video.Title)
		if len(keys) == 0 {
			continue
		}
		buckets := titlePrefixBuckets(keys, 12)
		if len(buckets) == 0 {
			continue
		}
		candidates = append(candidates, tagClusterCandidate{
			video:   video,
			keys:    keys,
			qgrams:  titleQGrams(keys, 4),
			buckets: buckets,
		})
	}
	if len(candidates) < 2 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].video.ID < candidates[j].video.ID
	})

	sets := newVideoMaintenanceDisjointSet(len(candidates))
	bucketIndex := make(map[string][]int)
	seenPairs := make(map[uint64]struct{})
	for i, right := range candidates {
		for _, bucket := range right.buckets {
			for _, j := range bucketIndex[bucket] {
				pairKey := videoMaintenancePairKey(i, j)
				if _, ok := seenPairs[pairKey]; ok {
					continue
				}
				seenPairs[pairKey] = struct{}{}
				left := candidates[j]
				if !tagClusterTitlePrefilter(left, right) {
					continue
				}
				if mediasim.TitleSimilarity(left.video.Title, right.video.Title) >= tagClusterTitleThreshold {
					sets.union(i, j)
				}
			}
		}
		for _, bucket := range right.buckets {
			bucketIndex[bucket] = append(bucketIndex[bucket], i)
		}
	}

	groupByRoot := make(map[int][]*catalog.Video)
	for i, candidate := range candidates {
		root := sets.find(i)
		groupByRoot[root] = append(groupByRoot[root], candidate.video)
	}
	roots := make([]int, 0, len(groupByRoot))
	for root, group := range groupByRoot {
		if len(group) >= 2 {
			roots = append(roots, root)
		}
	}
	sort.Ints(roots)
	groups := make([][]*catalog.Video, 0, len(roots))
	for _, root := range roots {
		groups = append(groups, groupByRoot[root])
	}
	return groups
}

func tagClusterTitlePrefilter(left, right tagClusterCandidate) bool {
	if !titleLengthCouldReachThreshold(left.keys, right.keys, tagClusterTitleThreshold) {
		return false
	}
	return qGramContainment(left.qgrams, right.qgrams) >= 0.45
}

func (a *App) propagateTagsAcrossTitleClusters(ctx context.Context) (int, error) {
	videos, err := a.cat.ListVideoMaintenanceCandidates(ctx)
	if err != nil {
		return 0, err
	}
	manualIDs, err := a.cat.ListManualTagVideoIDs(ctx)
	if err != nil {
		return 0, err
	}
	groups := collectTagTitleClusters(videos)
	addedTotal := 0
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return addedTotal, err
		}
		votes := make(map[string]int)
		canonical := make(map[string]string)
		for _, video := range group {
			seen := make(map[string]bool)
			for _, label := range video.Tags {
				label = strings.TrimSpace(label)
				key := strings.ToLower(label)
				if key == "" || seen[key] {
					continue
				}
				seen[key] = true
				votes[key]++
				if canonical[key] == "" {
					canonical[key] = label
				}
			}
		}
		eligible := make([]string, 0)
		for key, count := range votes {
			if count >= 2 && float64(count)/float64(len(group)) >= 0.5 {
				eligible = append(eligible, key)
			}
		}
		sort.Strings(eligible)
		if len(eligible) == 0 {
			continue
		}
		for _, video := range group {
			if manualIDs[video.ID] {
				continue
			}
			existing := make(map[string]bool, len(video.Tags))
			for _, label := range video.Tags {
				existing[strings.ToLower(strings.TrimSpace(label))] = true
			}
			assignments := make([]catalog.TagAssignment, 0, len(eligible))
			for _, key := range eligible {
				if existing[key] {
					continue
				}
				assignments = append(assignments, catalog.TagAssignment{
					Label:    canonical[key],
					Source:   "propagated",
					Evidence: "标题聚类",
				})
			}
			added, err := a.cat.AddVideoTagAssignments(ctx, video.ID, assignments)
			if err != nil {
				return addedTotal, err
			}
			addedTotal += added
		}
	}
	return addedTotal, nil
}
