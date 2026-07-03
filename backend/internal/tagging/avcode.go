package tagging

import (
	"regexp"
	"sort"
	"strings"
)

// 番号（AV code）识别。历史上散落在 catalog/tags.go，现在统一收敛到 tagging 包：
// 识别结果有两个用途——
//  1. 命中番号的视频归并到总标签 "AV"（避免每个番号变成独立标签）；
//  2. 从番号里提取"车牌前缀"（如 ABP-123 → ABP）作为系列信号，供夜间任务
//     给同系列视频建 series 标签。
var (
	knownAVSeriesPrefixes = []string{
		"SSNI", "SSIS", "SNIS", "SOE", "IPX", "IPZ", "IPTD",
		"ABP", "ABW", "ONEZ", "MIDE", "MIDV", "MIAA", "MIMK",
		"ATID", "SHKD", "RBD", "FSDSS", "STAR", "MUD", "HND",
		"HMN", "WANZ", "CREAM", "VAGU", "JUL", "JUQ", "JUR",
		"OBA", "NKK", "JUFE", "FC2PPV", "SIRO", "300MIUM",
		"259LUXU", "CAWD", "SABA", "ZIZ", "PPPD", "EBOD",
		"EBWH", "BOBB", "CJOD", "PRED", "VEC", "IBW", "LBJ",
		"IMPA", "DDK", "MVG", "HUNT", "NTRD", "SDDE", "DASS",
		"MKMP", "BF", "BFDM", "CARIB",
	}
	knownAVSeriesPrefixPattern = buildKnownAVSeriesPrefixPattern()
	knownAVCodePattern         = regexp.MustCompile(`(?i)^(?:` + knownAVSeriesPrefixPattern + `)[-_ ]?\d{2,8}(?:[-_ ]?[A-Z0-9]{1,4}){0,2}$`)
	avCodePattern              = regexp.MustCompile(`(?i)^[A-Z]{2,8}[-_ ]?\d{3,6}(?:[-_ ]?[A-Z0-9]{1,4})?$`)
	ccAVCodePattern            = regexp.MustCompile(`(?i)^CC[-_ ]?\d{3,8}(?:[-_ ]?[A-Z0-9]{1,4})?$`)
	fc2PPVAVCodePattern        = regexp.MustCompile(`(?i)^FC2[-_ ]?PPV[-_ ]?\d{4,8}(?:[-_ ]?[A-Z0-9]{1,4})?$`)
	fc2AVCodePattern           = regexp.MustCompile(`(?i)^FC2[-_ ]?\d{4,8}(?:[-_ ]?[A-Z0-9]{1,4})?$`)
	numericPrefixAVCodePattern = regexp.MustCompile(`(?i)^\d{2,4}[A-Z]{2,8}[-_ ]?\d{3,6}(?:[-_ ]?[A-Z0-9]{1,4})?$`)
	knownAVCodeInTextPattern   = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])((?:(?:` + knownAVSeriesPrefixPattern + `)[-_ ]?\d{2,8}(?:[-_ ]?[A-Z0-9]{1,4}){0,2}))(?:$|[^A-Za-z0-9])`)
	avCodeInTextPattern        = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])((?:(?:` + knownAVSeriesPrefixPattern + `)[-_ ]?\d{2,8}(?:[-_ ]?[A-Z0-9]{1,4}){0,2})|(?:[A-Z]{2,8}[-_ ]?\d{3,6}(?:[-_ ]?[A-Z0-9]{1,4})?)|(?:CC[-_ ]?\d{3,8}(?:[-_ ]?[A-Z0-9]{1,4})?)|(?:FC2[-_ ]?(?:PPV[-_ ]?)?\d{4,8}(?:[-_ ]?[A-Z0-9]{1,4})?)|(?:\d{2,4}[A-Z]{2,8}[-_ ]?\d{3,6}(?:[-_ ]?[A-Z0-9]{1,4})?))(?:$|[^A-Za-z0-9])`)

	seriesLettersPattern = regexp.MustCompile(`(?i)^\d{0,4}([A-Z]{2,8})[-_ ]?\d`)
)

// IsAVCode 判断一个独立字符串是否是番号。
func IsAVCode(label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	return avCodePattern.MatchString(label) ||
		knownAVCodePattern.MatchString(label) ||
		ccAVCodePattern.MatchString(label) ||
		fc2PPVAVCodePattern.MatchString(label) ||
		fc2AVCodePattern.MatchString(label) ||
		numericPrefixAVCodePattern.MatchString(label)
}

// ContainsAVCode 判断文本中是否出现番号。
func ContainsAVCode(text string) bool {
	return avCodeInTextPattern.MatchString(text)
}

// FindAVCode 返回文本中出现的第一个番号（原样片段），没有则返回空串。
func FindAVCode(text string) string {
	if m := knownAVCodeInTextPattern.FindStringSubmatch(text); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	m := avCodeInTextPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// SeriesOf 从一个番号中提取"车牌前缀"（系列名），统一为大写。
// 例：ABP-123 → ABP；390JAC-233 → JAC；FC2-PPV-1234567 → FC2PPV。
// 无法提取时返回空串。
func SeriesOf(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || !IsAVCode(code) {
		return ""
	}
	if series, ok := knownSeriesOf(code); ok {
		return series
	}
	if fc2PPVAVCodePattern.MatchString(code) {
		return "FC2PPV"
	}
	if fc2AVCodePattern.MatchString(code) {
		return "FC2"
	}
	m := seriesLettersPattern.FindStringSubmatch(code)
	if len(m) < 2 {
		return ""
	}
	return strings.ToUpper(m[1])
}

// SeriesInText 提取文本中第一个番号的系列前缀。
func SeriesInText(text string) string {
	return SeriesOf(FindAVCode(text))
}

func buildKnownAVSeriesPrefixPattern() string {
	prefixes := sortedKnownAVSeriesPrefixes()
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix == "FC2PPV" {
			parts = append(parts, `FC2[-_ ]?PPV`)
			continue
		}
		parts = append(parts, regexp.QuoteMeta(prefix))
	}
	return strings.Join(parts, "|")
}

func sortedKnownAVSeriesPrefixes() []string {
	prefixes := append([]string(nil), knownAVSeriesPrefixes...)
	sort.Slice(prefixes, func(i, j int) bool {
		if len(prefixes[i]) == len(prefixes[j]) {
			return prefixes[i] < prefixes[j]
		}
		return len(prefixes[i]) > len(prefixes[j])
	})
	return prefixes
}

func knownSeriesOf(code string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	normalized = strings.NewReplacer("_", "-", " ", "-").Replace(normalized)
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	if hasKnownSeriesPrefix(normalized, "FC2PPV") || hasKnownSeriesPrefix(normalized, "FC2-PPV") {
		return "FC2PPV", true
	}
	for _, prefix := range sortedKnownAVSeriesPrefixes() {
		if prefix == "FC2PPV" {
			continue
		}
		if hasKnownSeriesPrefix(normalized, prefix) {
			return prefix, true
		}
	}
	return "", false
}

func hasKnownSeriesPrefix(code, prefix string) bool {
	if !strings.HasPrefix(code, prefix) {
		return false
	}
	if len(code) == len(prefix) {
		return false
	}
	next := code[len(prefix)]
	return next == '-' || (next >= '0' && next <= '9')
}
