package tagging

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Rule 是单个标签的匹配规则，持久化在 tags.match_rules JSON 列。
//
//   - Keywords：子串匹配。适合 ≥2 字的中文词和较长的英文词（如"翘臀"、"blowjob"）。
//     为防误伤，单字 / ≤3 字符的纯 ASCII 词即使写进 Keywords 也会按整词处理。
//   - Words：整词（整段）匹配。文本先按"字母/数字连续段"切分，Words 里的词只有
//     和某个完整段相等才算命中。适合 "臀"、"ass"、"3p" 这类高误伤短词。
//   - Excludes：排除词。命中排除词的那部分文本会被抹除后再做匹配，例如
//     "人妻" 配置排除词"老婆饼"后，"老婆饼测评"不会再因"老婆"命中。
//   - MatchAVCode：识别文本中的番号（如 ABP-123、FC2-PPV-1234567）。通常由 AV
//     归并标签开启；其它标签一般不需要。
type Rule struct {
	Keywords    []string `json:"keywords,omitempty"`
	Words       []string `json:"words,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
	MatchAVCode bool     `json:"matchAvCode,omitempty"`
}

// IsEmpty 表示该规则没有任何显式配置（此时匹配器退化为按 label+aliases 匹配）。
func (r Rule) IsEmpty() bool {
	return len(r.Keywords) == 0 && len(r.Words) == 0 && len(r.Excludes) == 0 && !r.MatchAVCode
}

// RuleFromAliases 把"标签名 + 展示别名"转换成规则：单字与 ≤3 字符 ASCII 词
// 进 Words（整词），其余进 Keywords（子串）。用于没有显式规则的存量/用户标签，
// 行为与旧版子串匹配基本一致，但补上了短词保护。
func RuleFromAliases(label string, aliases []string) Rule {
	var rule Rule
	seen := map[string]struct{}{}
	for _, candidate := range append([]string{label}, aliases...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if isWholeWordOnly(key) {
			rule.Words = append(rule.Words, candidate)
		} else {
			rule.Keywords = append(rule.Keywords, candidate)
		}
	}
	return rule
}

// TagRule 是编译输入：一个标签名及其规则。
type TagRule struct {
	Label string
	Rule  Rule
}

// Field 是一段带名字的待匹配文本（如 标题/文件名/作者/目录）。
type Field struct {
	Name string
	Text string
}

// Match 是一次命中：Label 命中的标签，Field/Term 记录证据（在哪个字段命中了哪个词）。
type Match struct {
	Label string
	Field string
	Term  string
}

// Evidence 返回可读的命中证据，如 "文件名:翘臀"。
func (m Match) Evidence() string {
	if m.Field == "" {
		return m.Term
	}
	return m.Field + ":" + m.Term
}

type compiledTerm struct {
	raw     string // 原词（用于证据展示）
	lower   string
	compact string
}

type compiledRule struct {
	label       string
	keywords    []compiledTerm // 子串匹配
	words       []compiledTerm // 整词（整段）匹配
	excludes    []compiledTerm
	matchAVCode bool
}

// Matcher 是把全部标签规则一次性编译后的匹配器。构建一次可对任意多段文本
// 复用，避免旧实现里"每个文件查一遍全量标签"的开销。
type Matcher struct {
	rules []compiledRule
}

// NewMatcher 编译标签规则。空规则的标签会被跳过（调用方应先用 RuleFromAliases 兜底）。
func NewMatcher(tagRules []TagRule) *Matcher {
	m := &Matcher{rules: make([]compiledRule, 0, len(tagRules))}
	seen := map[string]struct{}{}
	for _, tr := range tagRules {
		label := strings.TrimSpace(tr.Label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cr := compiledRule{label: label, matchAVCode: tr.Rule.MatchAVCode}
		for _, kw := range tr.Rule.Keywords {
			term, ok := compileTerm(kw)
			if !ok {
				continue
			}
			// 短词保护：单字 / 短 ASCII 词强制走整词，即使配置在 Keywords 里。
			if isWholeWordOnly(term.lower) {
				cr.words = append(cr.words, term)
			} else {
				cr.keywords = append(cr.keywords, term)
			}
		}
		for _, w := range tr.Rule.Words {
			if term, ok := compileTerm(w); ok {
				cr.words = append(cr.words, term)
			}
		}
		for _, ex := range tr.Rule.Excludes {
			if term, ok := compileTerm(ex); ok {
				cr.excludes = append(cr.excludes, term)
			}
		}
		if len(cr.keywords) == 0 && len(cr.words) == 0 && !cr.matchAVCode {
			continue
		}
		m.rules = append(m.rules, cr)
	}
	return m
}

// Labels 返回编译进匹配器的全部标签名（按规则顺序）。
func (m *Matcher) Labels() []string {
	out := make([]string, 0, len(m.rules))
	for _, r := range m.rules {
		out = append(out, r.label)
	}
	return out
}

// Match 依次在各字段上运行全部规则，返回去重后的命中列表（每个标签只保留
// 第一处证据）。字段顺序即证据优先级，调用方应把"标题"放在最前。
func (m *Matcher) Match(fields ...Field) []Match {
	if m == nil || len(m.rules) == 0 {
		return nil
	}
	norms := make([]normText, 0, len(fields))
	for _, f := range fields {
		norms = append(norms, normalizeText(f.Text))
	}
	var out []Match
	matched := map[string]struct{}{}
	for _, rule := range m.rules {
		for i, f := range fields {
			if strings.TrimSpace(f.Text) == "" {
				continue
			}
			term, ok := rule.matchField(f.Text, norms[i])
			if !ok {
				continue
			}
			if _, dup := matched[strings.ToLower(rule.label)]; !dup {
				matched[strings.ToLower(rule.label)] = struct{}{}
				out = append(out, Match{Label: rule.label, Field: f.Name, Term: term})
			}
			break
		}
	}
	return out
}

// MatchLabels 是 Match 的便捷包装：单段匿名文本，只要标签名列表。
func (m *Matcher) MatchLabels(text string) []string {
	matches := m.Match(Field{Text: text})
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.Label)
	}
	return out
}

func (r compiledRule) matchField(raw string, norm normText) (string, bool) {
	if r.matchAVCode {
		if code := FindAVCode(raw); code != "" {
			return code, true
		}
	}
	if len(r.keywords) == 0 && len(r.words) == 0 {
		return "", false
	}
	effective := norm
	if len(r.excludes) > 0 {
		effective = norm.masked(r.excludes)
	}
	for _, kw := range r.keywords {
		if strings.Contains(effective.lower, kw.lower) || strings.Contains(effective.compact, kw.compact) {
			return kw.raw, true
		}
	}
	for _, w := range r.words {
		if _, ok := effective.segments[w.compact]; ok {
			return w.raw, true
		}
	}
	return "", false
}

// ---------- 文本归一化 ----------

type normText struct {
	lower    string
	compact  string
	segments map[string]struct{}
}

func normalizeText(s string) normText {
	lower := strings.ToLower(s)
	return normText{
		lower:    lower,
		compact:  compactText(lower),
		segments: segmentsOf(lower),
	}
}

// masked 返回抹掉排除词后的文本视图：排除词在 lower 里替换成空格后重新计算
// compact 与 segments，另外在 compact 上抹掉排除词的 compact 形态（覆盖排除词
// 自身带分隔符的情况）。
func (n normText) masked(excludes []compiledTerm) normText {
	lower := n.lower
	for _, ex := range excludes {
		if ex.lower != "" && strings.Contains(lower, ex.lower) {
			lower = strings.ReplaceAll(lower, ex.lower, " ")
		}
	}
	compact := compactText(lower)
	for _, ex := range excludes {
		if ex.compact != "" && strings.Contains(compact, ex.compact) {
			compact = strings.ReplaceAll(compact, ex.compact, " ")
		}
	}
	return normText{lower: lower, compact: compact, segments: segmentsOf(lower)}
}

func compileTerm(s string) (compiledTerm, bool) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return compiledTerm{}, false
	}
	lower := strings.ToLower(raw)
	compact := compactText(lower)
	if compact == "" {
		return compiledTerm{}, false
	}
	return compiledTerm{raw: raw, lower: lower, compact: compact}, true
}

func compactText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func segmentsOf(lower string) map[string]struct{} {
	segments := map[string]struct{}{}
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			segments[b.String()] = struct{}{}
			b.Reset()
		}
	}
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return segments
}

// isWholeWordOnly 判定一个词是否必须整词匹配：
//   - 单个字符（含单个 CJK 字，如"臀"）
//   - ≤3 字符且不含分隔符的纯 ASCII 词（如 "av"、"3p"、"ass"）
func isWholeWordOnly(lower string) bool {
	compact := compactText(lower)
	if compact == "" {
		return false
	}
	if utf8.RuneCountInString(compact) == 1 {
		return true
	}
	if len(compact) <= 3 && compact == lower && isASCIIAlnum(compact) {
		return true
	}
	return false
}

func isASCIIAlnum(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
