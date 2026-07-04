// Package fixedtags 是内置标签规则包：
//
//   - builtin 标签保留 AV、奶子、女大、人妻、后入、制服、美臀、口交。
//   - 内置标签由启动/维护流程确保存在；删除后不会留下标签墓碑。
//   - AV 是特例：用户删除后由 catalog 设置关闭番号匹配机制，不再自动补回。
//
// 每个标签携带 tagging.Rule 匹配规则（子串词 / 整词 / 排除词），首次入库时
// 写进 tags.match_rules；之后管理员在后台的修改优先，这里的默认值不会覆盖。
package fixedtags

import (
	"strings"

	"github.com/video-site/backend/internal/tagging"
)

const SourceBuiltin = "builtin"

// Tag 是一条内置标签定义。Aliases 仅作展示同义词；实际匹配走 Rule。
type Tag struct {
	Label   string
	Source  string
	Aliases []string
	Rule    tagging.Rule
}

// Labels 保留旧包变量名：当前允许保留为 builtin 的全部内置标签名。
var Labels = []string{"AV", "奶子", "女大", "人妻", "后入", "制服", "美臀", "口交"}

var builtinTags = []Tag{
	{
		Label:  "AV",
		Source: SourceBuiltin,
		Rule:   tagging.Rule{MatchAVCode: true},
	},
	{
		Label:  "奶子",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"奶子", "大奶", "巨乳", "美乳", "爆乳", "丰乳", "丰胸", "大胸", "胸部", "胸器",
				"揉胸", "揉奶", "揉乳", "双乳", "乳房", "乳头", "美胸",
				"boobs", "big boobs", "tits", "titties", "titty", "breast", "breasts", "boob",
			},
			Words:    []string{"奶", "胸"},
			Excludes: []string{"奶奶", "牛奶", "奶茶", "酸奶"},
		},
	},
	{
		Label:  "女大",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"女大", "女大学生", "大学生", "女子大生", "女学生", "学生妹", "校花", "学妹", "校园",
				"college student", "university student",
			},
			Words:    []string{"大学", "大一", "大二", "大三", "大四", "college", "campus", "coed"},
			Excludes: []string{"大学路"},
		},
	},
	{
		Label:  "人妻",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"人妻", "妻子", "老婆", "太太", "少妇", "已婚", "良家", "人妇",
				"housewife", "married woman", "young wife",
			},
			Words:    []string{"wife", "married"},
			Excludes: []string{"老婆饼"},
		},
	},
	{
		Label:  "后入",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"后入", "後入", "后入式", "後入式", "后进", "後進", "后位", "後位",
				"背入", "背后式", "后背位", "狗爬", "狗爬式",
				"doggy", "doggystyle", "doggy style", "backshot", "back shot", "from behind", "rear entry",
			},
		},
	},
	{
		Label:  "制服",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{"制服", "水手服", "空姐", "护士", "女仆", "秘书", "女教师", "警花", "旗袍", "JK制服"},
			Words:    []string{"jk", "ol"},
		},
	},
	{
		Label:  "美臀",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"屁股", "屁屁", "翘臀", "美臀", "肥臀", "巨臀", "蜜桃臀", "大屁股",
				"后庭", "後庭", "肛交", "屁眼", "菊花",
				"booty", "buttocks", "big ass", "big butt",
			},
			Words:    []string{"臀", "尻", "肛", "ass", "butt", "hip"},
			Excludes: []string{"菊花茶"},
		},
	},
	{
		Label:  "口交",
		Source: SourceBuiltin,
		Rule: tagging.Rule{
			Keywords: []string{
				"口交", "口爆", "口活", "口射", "吹箫", "吹萧", "深喉", "吞精",
				"含屌", "含鸡巴", "含龟头", "舔屌",
				"blowjob", "blow job", "oral sex", "oral-sex", "oralsex", "fellatio",
			},
			Words: []string{"bj", "oral"},
		},
	},
}

// All 返回全部内置标签定义。返回副本，调用方可安全修改。
func All() []Tag {
	out := make([]Tag, len(builtinTags))
	copy(out, builtinTags)
	return out
}

// IsBuiltinLabel reports whether label is one of the current builtin labels.
func IsBuiltinLabel(label string) bool {
	label = strings.TrimSpace(label)
	for _, builtin := range Labels {
		if strings.EqualFold(label, builtin) {
			return true
		}
	}
	return false
}

// RuleFor 返回某个内置标签的默认规则；不存在时返回零值。
func RuleFor(label string) tagging.Rule {
	for _, t := range All() {
		if t.Label == label {
			return t.Rule
		}
	}
	return tagging.Rule{}
}

// AliasesFor 保留旧包函数名：返回标签的展示别名（现在默认与规则分离，通常为空）。
func AliasesFor(label string) []string {
	for _, t := range All() {
		if t.Label == label {
			return append([]string(nil), t.Aliases...)
		}
	}
	return nil
}
