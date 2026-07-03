package fixedtags

import (
	"testing"

	"github.com/video-site/backend/internal/tagging"
)

func packMatcher(t *testing.T) *tagging.Matcher {
	t.Helper()
	var rules []tagging.TagRule
	for _, tag := range All() {
		rules = append(rules, tagging.TagRule{Label: tag.Label, Rule: tag.Rule})
	}
	return tagging.NewMatcher(rules)
}

func TestPackMatchesEnglishTerms(t *testing.T) {
	m := packMatcher(t)
	got := m.MatchLabels("backshot oral-sex big boobs big ass wife college student.mp4")
	want := map[string]bool{"后入": true, "口交": true, "奶子": true, "美臀": true, "人妻": true, "女大": true}
	assertLabelSet(t, got, want)
}

func TestPackMatchesChineseTerms(t *testing.T) {
	m := packMatcher(t)
	got := m.MatchLabels("背后式揉乳口活蜜桃臀少妇大学生.mp4")
	want := map[string]bool{"后入": true, "奶子": true, "口交": true, "美臀": true, "人妻": true, "女大": true}
	assertLabelSet(t, got, want)
}

func TestPackSingleCharProtection(t *testing.T) {
	m := packMatcher(t)
	// "牛奶"、"胸怀"：单字 "奶"/"胸" 不应子串误伤。
	if got := m.MatchLabels("牛奶广告拍摄花絮"); len(got) != 0 {
		t.Fatalf("误伤: %#v", got)
	}
}

func TestPackBuiltinTags(t *testing.T) {
	m := packMatcher(t)
	cases := map[string]string{
		"JK 制服少女":  "制服",
		"高冷空姐":     "制服",
		"SSNI-001": "AV",
	}
	for text, want := range cases {
		got := m.MatchLabels(text)
		found := false
		for _, label := range got {
			if label == want {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchLabels(%q) = %#v, want contains %q", text, got, want)
		}
	}
}

func TestPackAVDoesNotMatchPlainAliasText(t *testing.T) {
	m := packMatcher(t)
	for _, text := range []string{"经典 AV 合集", "JAV合集", "番号整理", "番號整理"} {
		if got := m.MatchLabels(text); len(got) != 0 {
			t.Fatalf("MatchLabels(%q) = %#v, want none", text, got)
		}
	}
}

func TestAllHasNoDuplicateLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, tag := range All() {
		if seen[tag.Label] {
			t.Fatalf("duplicate builtin label %q", tag.Label)
		}
		seen[tag.Label] = true
		if tag.Source != SourceBuiltin {
			t.Fatalf("tag %q source = %q, want %q", tag.Label, tag.Source, SourceBuiltin)
		}
		if tag.Rule.IsEmpty() {
			t.Fatalf("tag %q has empty rule", tag.Label)
		}
	}
	for _, label := range Labels {
		if !seen[label] {
			t.Fatalf("core builtin label %q missing from All()", label)
		}
	}
	if len(seen) != len(Labels) {
		t.Fatalf("builtin labels = %#v, want exactly %#v", seen, Labels)
	}
}

func assertLabelSet(t *testing.T, got []string, want map[string]bool) {
	t.Helper()
	gotSet := map[string]bool{}
	for _, label := range got {
		gotSet[label] = true
	}
	for label := range want {
		if !gotSet[label] {
			t.Errorf("missing label %q in %#v", label, got)
		}
	}
}
