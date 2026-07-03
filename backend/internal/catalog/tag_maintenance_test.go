package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/video-site/backend/internal/tagging"
)

func openTagMaintenanceTestCatalog(t *testing.T) (*Catalog, context.Context) {
	t.Helper()
	cat, err := Open(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() {
		if err := cat.Close(); err != nil {
			t.Fatalf("close catalog: %v", err)
		}
	})
	return cat, context.Background()
}

func seedTagMaintenanceVideo(t *testing.T, cat *Catalog, id, title, fileName string) {
	t.Helper()
	now := time.Now()
	if err := cat.UpsertVideo(context.Background(), &Video{
		ID:          id,
		DriveID:     "drive",
		FileID:      "file-" + id,
		FileName:    fileName,
		Title:       title,
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed video %s: %v", id, err)
	}
}

func TestReplaceAutoVideoTagsPreservesIndependentSourcesAndManualLock(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	seedTagMaintenanceVideo(t, cat, "replace", "ordinary", "ordinary.mp4")
	seedTagMaintenanceVideo(t, cat, "manual", "ordinary", "manual.mp4")

	for _, label := range []string{"old-auto", "new-auto", "crawler-tag", "manual-tag"} {
		if _, err := cat.EnsureTag(ctx, label, "user"); err != nil {
			t.Fatalf("ensure %s: %v", label, err)
		}
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "replace", []TagAssignment{
		{Label: "old-auto", Source: "legacy", Evidence: "old"},
		{Label: "crawler-tag", Source: "crawler", Evidence: "script"},
	}); err != nil {
		t.Fatalf("seed assignments: %v", err)
	}
	if _, err := cat.ReplaceAutoVideoTags(ctx, "replace", []TagAssignment{
		{Label: "new-auto", Source: "auto", Evidence: "标题:new-auto"},
	}); err != nil {
		t.Fatalf("replace auto tags: %v", err)
	}
	got, err := cat.GetVideo(ctx, "replace")
	if err != nil {
		t.Fatalf("get replace video: %v", err)
	}
	if !sameStrings(got.Tags, []string{"new-auto", "crawler-tag"}) {
		t.Fatalf("tags = %#v, want new-auto + crawler-tag", got.Tags)
	}
	metadata, err := cat.ListVideoTagMetadata(ctx, []string{"replace"})
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if metadata["replace"]["crawler-tag"].Source != "crawler" || metadata["replace"]["new-auto"].Source != "auto" {
		t.Fatalf("metadata = %#v", metadata["replace"])
	}

	if err := cat.SetManualVideoTags(ctx, "manual", []string{"manual-tag"}); err != nil {
		t.Fatalf("lock manual video: %v", err)
	}
	if _, err := cat.ReplaceAutoVideoTags(ctx, "manual", []TagAssignment{{Label: "new-auto", Source: "auto"}}); err != nil {
		t.Fatalf("replace locked video: %v", err)
	}
	locked, err := cat.GetVideo(ctx, "manual")
	if err != nil {
		t.Fatalf("get manual video: %v", err)
	}
	if !sameStrings(locked.Tags, []string{"manual-tag"}) {
		t.Fatalf("manual tags = %#v, want unchanged", locked.Tags)
	}

	if _, err := cat.db.ExecContext(ctx, `UPDATE videos SET updated_at = 123 WHERE id = 'replace'`); err != nil {
		t.Fatalf("set stable timestamp: %v", err)
	}
	if _, err := cat.ReplaceAutoVideoTags(ctx, "replace", []TagAssignment{{Label: "new-auto", Source: "auto"}}); err != nil {
		t.Fatalf("idempotent replace: %v", err)
	}
	var updatedAt int64
	if err := cat.db.QueryRowContext(ctx, `SELECT updated_at FROM videos WHERE id = 'replace'`).Scan(&updatedAt); err != nil {
		t.Fatalf("read timestamp: %v", err)
	}
	if updatedAt != 123 {
		t.Fatalf("idempotent replacement updated video timestamp to %d", updatedAt)
	}
}

func TestReplaceAutoVideoTagsRespectsSourcePriorityAndRefreshesEvidence(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	seedTagMaintenanceVideo(t, cat, "priority", "source-tag clip", "priority.mp4")
	seedTagMaintenanceVideo(t, cat, "evidence", "source-tag clip", "evidence.mp4")
	if _, err := cat.EnsureTag(ctx, "source-tag", "user"); err != nil {
		t.Fatalf("ensure source tag: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "priority", []TagAssignment{{
		Label: "source-tag", Source: "crawler", Evidence: "脚本标签",
	}}); err != nil {
		t.Fatalf("seed crawler tag: %v", err)
	}
	if _, err := cat.db.ExecContext(ctx, `UPDATE videos SET updated_at = 321 WHERE id = 'priority'`); err != nil {
		t.Fatalf("set priority timestamp: %v", err)
	}
	changed, err := cat.ReplaceAutoVideoTags(ctx, "priority", []TagAssignment{{
		Label: "source-tag", Source: "auto", Evidence: "标题:source-tag",
	}})
	if err != nil {
		t.Fatalf("replace priority auto: %v", err)
	}
	if changed {
		t.Fatal("auto replacement changed a crawler-owned tag")
	}
	var updatedAt int64
	if err := cat.db.QueryRowContext(ctx, `SELECT updated_at FROM videos WHERE id = 'priority'`).Scan(&updatedAt); err != nil {
		t.Fatalf("read priority timestamp: %v", err)
	}
	if updatedAt != 321 {
		t.Fatalf("priority timestamp = %d, want unchanged", updatedAt)
	}
	metadata, err := cat.ListVideoTagMetadata(ctx, []string{"priority"})
	if err != nil {
		t.Fatalf("priority metadata: %v", err)
	}
	if got := metadata["priority"]["source-tag"]; got.Source != "crawler" || got.Evidence != "脚本标签" {
		t.Fatalf("priority metadata = %#v, want crawler evidence", got)
	}

	if _, err := cat.ReplaceAutoVideoTags(ctx, "evidence", []TagAssignment{{
		Label: "source-tag", Source: "auto", Evidence: "标题:source-tag",
	}}); err != nil {
		t.Fatalf("seed auto evidence: %v", err)
	}
	changed, err = cat.ReplaceAutoVideoTags(ctx, "evidence", []TagAssignment{{
		Label: "source-tag", Source: "auto", Evidence: "文件名:source-tag",
	}})
	if err != nil {
		t.Fatalf("refresh auto evidence: %v", err)
	}
	if !changed {
		t.Fatal("evidence refresh was not reported as a change")
	}
	metadata, err = cat.ListVideoTagMetadata(ctx, []string{"evidence"})
	if err != nil {
		t.Fatalf("evidence metadata: %v", err)
	}
	if got := metadata["evidence"]["source-tag"]; got.Source != "auto" || got.Evidence != "文件名:source-tag" {
		t.Fatalf("evidence metadata = %#v, want refreshed auto evidence", got)
	}
}

func TestRetagVideosBatchRemovesLegacyAndProtectsManualVideos(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	seedTagMaintenanceVideo(t, cat, "a-auto", "fresh-keyword clip", "a.mp4")
	seedTagMaintenanceVideo(t, cat, "b-manual", "fresh-keyword clip", "b.mp4")

	fresh, err := cat.EnsureTag(ctx, "fresh-keyword", "user")
	if err != nil {
		t.Fatalf("ensure fresh tag: %v", err)
	}
	stale, err := cat.EnsureTag(ctx, "stale-tag", "user")
	if err != nil {
		t.Fatalf("ensure stale tag: %v", err)
	}
	if err := cat.insertVideoTag(ctx, "a-auto", stale.ID, "legacy", "legacy"); err != nil {
		t.Fatalf("seed stale legacy: %v", err)
	}
	if err := cat.syncVideoTagsJSON(ctx, "a-auto", false); err != nil {
		t.Fatalf("sync stale legacy: %v", err)
	}
	if err := cat.SetManualVideoTags(ctx, "b-manual", []string{"stale-tag"}); err != nil {
		t.Fatalf("lock manual: %v", err)
	}

	matcher := tagging.NewMatcher([]tagging.TagRule{{
		Label: fresh.Label,
		Rule:  tagging.Rule{Keywords: []string{"fresh-keyword"}},
	}})
	processed, lastID, done, err := cat.RetagVideosBatch(ctx, matcher, "", 10, 0)
	if err != nil {
		t.Fatalf("retag: %v", err)
	}
	if processed != 2 || lastID != "b-manual" || !done {
		t.Fatalf("retag result = %d/%q/%v", processed, lastID, done)
	}
	autoVideo, _ := cat.GetVideo(ctx, "a-auto")
	if !sameStrings(autoVideo.Tags, []string{"fresh-keyword"}) {
		t.Fatalf("auto tags = %#v", autoVideo.Tags)
	}
	manualVideo, _ := cat.GetVideo(ctx, "b-manual")
	if !sameStrings(manualVideo.Tags, []string{"stale-tag"}) {
		t.Fatalf("manual tags = %#v", manualVideo.Tags)
	}

	processed, _, done, err = cat.RetagVideosBatch(ctx, matcher, "", 10, 0)
	if err != nil || processed != 2 || !done {
		t.Fatalf("idempotent retag = %d/%v/%v", processed, done, err)
	}
}

func TestResetGeneratedTagStateClearsGeneratedTagsAndTombstones(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	seedTagMaintenanceVideo(t, cat, "reset-auto", "ordinary", "reset-auto.mp4")
	seedTagMaintenanceVideo(t, cat, "reset-crawler", "ordinary", "reset-crawler.mp4")
	seedTagMaintenanceVideo(t, cat, "reset-propagated", "ordinary", "reset-propagated.mp4")

	for _, label := range []string{"auto-user", "propagated-user"} {
		if _, err := cat.EnsureTag(ctx, label, "user"); err != nil {
			t.Fatalf("ensure %s: %v", label, err)
		}
	}
	if _, err := cat.EnsureTag(ctx, "generated-manual", "generated"); err != nil {
		t.Fatalf("ensure generated-manual: %v", err)
	}
	if _, err := cat.EnsureTag(ctx, "SERIESRESET", "generated"); err != nil {
		t.Fatalf("ensure series: %v", err)
	}
	if _, err := cat.EnsureCrawlerTag(ctx, "Crawler Owner"); err != nil {
		t.Fatalf("ensure crawler: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "reset-auto", []TagAssignment{
		{Label: "auto-user", Source: "auto", Evidence: "auto"},
		{Label: "generated-manual", Source: "manual", Evidence: "manual"},
		{Label: "SERIESRESET", Source: "series", Evidence: "series"},
	}); err != nil {
		t.Fatalf("seed reset-auto assignments: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "reset-crawler", []TagAssignment{{
		Label: "Crawler Owner", Source: "crawler", Evidence: "crawler",
	}}); err != nil {
		t.Fatalf("seed crawler assignment: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "reset-propagated", []TagAssignment{{
		Label: "propagated-user", Source: "propagated", Evidence: "propagated",
	}}); err != nil {
		t.Fatalf("seed propagated assignment: %v", err)
	}
	if _, err := cat.db.ExecContext(ctx,
		`INSERT INTO deleted_tags (label, source, deleted_at) VALUES ('deleted-generated', 'generated', ?)`,
		time.Now().UnixMilli(),
	); err != nil {
		t.Fatalf("seed tombstone: %v", err)
	}

	result, err := cat.ResetGeneratedTagState(ctx)
	if err != nil {
		t.Fatalf("reset generated state: %v", err)
	}
	if result.RemovedTags != 2 || result.ClearedTombstones != 1 {
		t.Fatalf("reset result = %#v, want 2 tags and 1 tombstone", result)
	}
	resetAuto, _ := cat.GetVideo(ctx, "reset-auto")
	if len(resetAuto.Tags) != 0 {
		t.Fatalf("reset-auto tags = %#v, want none", resetAuto.Tags)
	}
	resetCrawler, _ := cat.GetVideo(ctx, "reset-crawler")
	if !sameStrings(resetCrawler.Tags, []string{"Crawler Owner"}) {
		t.Fatalf("crawler tags = %#v", resetCrawler.Tags)
	}
	resetPropagated, _ := cat.GetVideo(ctx, "reset-propagated")
	if !sameStrings(resetPropagated.Tags, []string{"propagated-user"}) {
		t.Fatalf("propagated tags = %#v", resetPropagated.Tags)
	}
	tags := mustListTags(t, ctx, cat)
	if hasTagLabel(tags, "generated-manual") || hasTagLabel(tags, "SERIESRESET") {
		t.Fatalf("ordinary generated tags were not removed: %#v", tags)
	}
	if !hasTagLabel(tags, "Crawler Owner") {
		t.Fatalf("crawler tag was removed: %#v", tags)
	}
	var tombstones int
	if err := cat.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deleted_tags`).Scan(&tombstones); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if tombstones != 0 {
		t.Fatalf("tombstones = %d, want 0", tombstones)
	}
}

func hasTagLabel(tags []Tag, label string) bool {
	for _, tag := range tags {
		if tag.Label == label {
			return true
		}
	}
	return false
}

func TestSyncSeriesTagsCreatesRevokesAndRespectsTombstone(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	for i, code := range []string{"ABP-101", "ABP-102", "ABP-103"} {
		seedTagMaintenanceVideo(t, cat, "series-"+string(rune('a'+i)), code, code+".mp4")
	}
	added, err := cat.SyncSeriesTags(ctx, 3)
	if err != nil {
		t.Fatalf("sync series: %v", err)
	}
	if added != 3 {
		t.Fatalf("series rows added = %d, want 3", added)
	}
	tag, err := cat.getTagByLabel(ctx, "ABP")
	if err != nil {
		t.Fatalf("get series tag: %v", err)
	}
	if tag.Source != "generated" {
		t.Fatalf("series tag source = %q, want generated", tag.Source)
	}

	if _, err := cat.db.ExecContext(ctx, `UPDATE videos SET title = 'other', file_name = 'other.mp4' WHERE id = 'series-c'`); err != nil {
		t.Fatalf("change series member: %v", err)
	}
	if _, err := cat.SyncSeriesTags(ctx, 3); err != nil {
		t.Fatalf("revoke series: %v", err)
	}
	for _, id := range []string{"series-a", "series-b", "series-c"} {
		video, _ := cat.GetVideo(ctx, id)
		for _, label := range video.Tags {
			if label == "ABP" {
				t.Fatalf("%s retained revoked ABP tag: %#v", id, video.Tags)
			}
		}
	}

	if _, err := cat.db.ExecContext(ctx, `UPDATE videos SET title = 'ABP-103', file_name = 'ABP-103.mp4' WHERE id = 'series-c'`); err != nil {
		t.Fatalf("restore series member: %v", err)
	}
	if _, err := cat.SyncSeriesTags(ctx, 3); err != nil {
		t.Fatalf("restore series: %v", err)
	}
	tag, err = cat.getTagByLabel(ctx, "ABP")
	if err != nil {
		t.Fatalf("get restored series tag: %v", err)
	}
	if _, err := cat.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("delete series tag: %v", err)
	}
	if added, err := cat.SyncSeriesTags(ctx, 3); err != nil || added != 0 {
		t.Fatalf("tombstoned series sync = %d, %v", added, err)
	}
	if _, err := cat.getTagByLabel(ctx, "ABP"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tombstoned series was recreated: %v", err)
	}
}

func TestSyncSeriesTagsRespectsAutoGenerateSetting(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	if err := cat.SetAutoGenerateTagsEnabled(ctx, false); err != nil {
		t.Fatalf("disable auto-generate tags: %v", err)
	}
	for i, code := range []string{"ABP-201", "ABP-202", "ABP-203"} {
		seedTagMaintenanceVideo(t, cat, "series-disabled-"+string(rune('a'+i)), code, code+".mp4")
	}
	if added, err := cat.SyncSeriesTags(ctx, 3); err != nil || added != 0 {
		t.Fatalf("disabled series sync = %d, %v; want 0, nil", added, err)
	}
	if _, err := cat.getTagByLabel(ctx, "ABP"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("disabled sync created ABP tag: %v", err)
	}

	if _, err := cat.EnsureTag(ctx, "ABP", "user"); err != nil {
		t.Fatalf("seed existing ABP tag: %v", err)
	}
	if added, err := cat.SyncSeriesTags(ctx, 3); err != nil || added != 3 {
		t.Fatalf("existing series sync = %d, %v; want 3, nil", added, err)
	}
	for i := range []string{"ABP-201", "ABP-202", "ABP-203"} {
		id := "series-disabled-" + string(rune('a'+i))
		video, err := cat.GetVideo(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if !hasTag(video.Tags, "ABP") {
			t.Fatalf("%s tags = %#v, want existing ABP", id, video.Tags)
		}
	}
}

func TestAVCodesGetAVAndKnownSeriesTags(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	codes := []string{"FC2PPV-3259498", "FC2PPV-4162750", "FC2PPV-4768873"}
	for i, code := range codes {
		id := "fc2ppv-" + string(rune('a'+i))
		seedTagMaintenanceVideo(t, cat, id, code, code+".mp4")
		assignments, err := cat.MatchTagAssignments(ctx, code, code+".mp4", "", "")
		if err != nil {
			t.Fatalf("match assignments for %s: %v", code, err)
		}
		if _, err := cat.ReplaceAutoVideoTags(ctx, id, assignments); err != nil {
			t.Fatalf("attach AV tag for %s: %v", id, err)
		}
	}
	if _, err := cat.EnsureTag(ctx, "FC2PPV", "generated"); err != nil {
		t.Fatalf("ensure FC2PPV tag: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "fc2ppv-a", []TagAssignment{{
		Label: "FC2PPV", Source: "auto", Evidence: "文件名:FC2PPV",
	}}); err != nil {
		t.Fatalf("seed auto FC2PPV source: %v", err)
	}
	if added, err := cat.SyncSeriesTags(ctx, 3); err != nil || added != 3 {
		t.Fatalf("sync FC2PPV series = %d, %v", added, err)
	}
	for i := range codes {
		id := "fc2ppv-" + string(rune('a'+i))
		video, err := cat.GetVideo(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if !hasTag(video.Tags, "AV") || !hasTag(video.Tags, "FC2PPV") {
			t.Fatalf("%s tags = %#v, want AV and FC2PPV", id, video.Tags)
		}
	}
	metadata, err := cat.ListVideoTagMetadata(ctx, []string{"fc2ppv-a"})
	if err != nil {
		t.Fatalf("FC2PPV metadata: %v", err)
	}
	if got := metadata["fc2ppv-a"]["FC2PPV"]; got.Source != "series" {
		t.Fatalf("FC2PPV source = %#v, want series", got)
	}
}

func TestDuplicatePropagationAndClearAreReversible(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	for _, id := range []string{"dup-a", "dup-b", "dup-manual", "dup-hidden"} {
		seedTagMaintenanceVideo(t, cat, id, id, id+".mp4")
		if _, err := cat.db.ExecContext(ctx,
			`UPDATE videos SET size_bytes = 99, sampled_sha256 = 'same-hash' WHERE id = ?`, id); err != nil {
			t.Fatalf("seed fingerprint %s: %v", id, err)
		}
	}
	if _, err := cat.db.ExecContext(ctx, `UPDATE videos SET hidden = 1 WHERE id = 'dup-hidden'`); err != nil {
		t.Fatalf("hide duplicate member: %v", err)
	}
	for _, label := range []string{"origin-tag", "manual-tag", "hidden-tag"} {
		if _, err := cat.EnsureTag(ctx, label, "user"); err != nil {
			t.Fatalf("ensure %s: %v", label, err)
		}
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "dup-a", []TagAssignment{{Label: "origin-tag", Source: "auto"}}); err != nil {
		t.Fatalf("seed origin: %v", err)
	}
	if err := cat.SetManualVideoTags(ctx, "dup-manual", []string{"manual-tag"}); err != nil {
		t.Fatalf("seed manual: %v", err)
	}
	if _, err := cat.AddVideoTagAssignments(ctx, "dup-hidden", []TagAssignment{{Label: "hidden-tag", Source: "auto"}}); err != nil {
		t.Fatalf("seed hidden origin: %v", err)
	}

	added, err := cat.PropagateTagsAcrossDuplicates(ctx)
	if err != nil {
		t.Fatalf("propagate duplicates: %v", err)
	}
	if added != 3 {
		t.Fatalf("propagated rows = %d, want 3", added)
	}
	recipient, _ := cat.GetVideo(ctx, "dup-b")
	if !sameStrings(recipient.Tags, []string{"origin-tag", "manual-tag"}) {
		t.Fatalf("recipient tags = %#v", recipient.Tags)
	}
	manual, _ := cat.GetVideo(ctx, "dup-manual")
	if !sameStrings(manual.Tags, []string{"manual-tag"}) {
		t.Fatalf("manual duplicate changed: %#v", manual.Tags)
	}

	affected, err := cat.ClearPropagatedTags(ctx)
	if err != nil {
		t.Fatalf("clear propagation: %v", err)
	}
	if affected != 2 {
		t.Fatalf("cleared videos = %d, want 2", affected)
	}
	recipient, _ = cat.GetVideo(ctx, "dup-b")
	if len(recipient.Tags) != 0 {
		t.Fatalf("recipient retained propagated tags: %#v", recipient.Tags)
	}
	origin, _ := cat.GetVideo(ctx, "dup-a")
	if !sameStrings(origin.Tags, []string{"origin-tag"}) {
		t.Fatalf("origin tag was cleared: %#v", origin.Tags)
	}
}

func TestUpdateTagClassifiesWithNewRuleAndPrunesOnlyAutomaticSources(t *testing.T) {
	cat, ctx := openTagMaintenanceTestCatalog(t)
	seedTagMaintenanceVideo(t, cat, "rule-video", "special phrase", "rule.mp4")
	userTag, err := cat.EnsureTag(ctx, "display-label", "user")
	if err != nil {
		t.Fatalf("ensure user tag: %v", err)
	}
	updated, err := cat.UpdateTag(ctx, userTag.ID, []string{"alias"}, tagging.Rule{Keywords: []string{"special phrase"}})
	if err != nil {
		t.Fatalf("update tag: %v", err)
	}
	if len(updated.MatchRules.Keywords) != 1 {
		t.Fatalf("updated rules = %#v", updated.MatchRules)
	}
	classified, err := cat.ClassifyTagByID(ctx, userTag.ID)
	if err != nil || classified != 1 {
		t.Fatalf("classify updated tag = %d, %v", classified, err)
	}
	video, _ := cat.GetVideo(ctx, "rule-video")
	if !sameStrings(video.Tags, []string{"display-label"}) {
		t.Fatalf("classified tags = %#v", video.Tags)
	}

	if _, err := cat.EnsureTag(ctx, "orphan-auto", "generated"); err != nil {
		t.Fatalf("ensure automatic orphan: %v", err)
	}
	if _, err := cat.EnsureTag(ctx, "orphan-user", "user"); err != nil {
		t.Fatalf("ensure user orphan: %v", err)
	}
	pruned, err := cat.PruneUnreferencedTags(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want generated orphan only", pruned)
	}
	if _, err := cat.getTagByLabel(ctx, "orphan-user"); err != nil {
		t.Fatalf("user orphan was pruned: %v", err)
	}
}

func hasTag(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
