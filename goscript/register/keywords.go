package register

// 代码补全关键字生成。
// 从已注册的外部包中提取所有导出对象，
// 生成编辑器代码补全所需的 KeywordInfo 列表。

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/importer"
)

// kindNames 将对象种类映射为显示名称。
var kindNames = map[importer.ObjectKind]string{
	importer.ObjectVar:         "Variable",
	importer.ObjectConst:       "Constant",
	importer.ObjectType:        "Struct",
	importer.ObjectFunc:        "Function",
	importer.ObjectBuiltinFunc: "Function",
}

// KeywordInfo 表示代码补全条目的信息。
type KeywordInfo struct {
	Label           string `json:"label"`           // 显示标签，如 "fmt.Println"
	Kind            string `json:"kind"`            // 对象种类，如 "Function"
	InsertText      string `json:"insertText"`      // 插入文本（可能包含参数占位符）
	InsertTextRules string `json:"insertTextRules"` // 插入规则，如 "InsertAsSnippet"
}

// Keywords 返回所有已注册包的代码补全关键字列表。
// 对于函数类型，会生成带参数占位符的代码片段（snippet）。
func Keywords() []*KeywordInfo {
	keywords := make([]*KeywordInfo, 0)

	for _, pkg := range importer.GetAllPackages() {
		for _, obj := range pkg.Objects {
			info := KeywordInfo{
				Label: fmt.Sprintf("%s.%s", pkg.Name, obj.Name),
				Kind:  kindNames[obj.Kind],
			}

			if info.Kind == "Function" {
				// 为函数构建带参数占位符的代码片段
				params := make([]string, 0)
				for i := 0; i < obj.Type.NumIn(); i++ {
					params = append(params, fmt.Sprintf("${%d:%s}", i+1, obj.Type.In(i).String()))
				}
				info.InsertText = fmt.Sprintf("%s(%s)", info.Label, strings.Join(params, ", "))
				info.InsertTextRules = "InsertAsSnippet"
			} else {
				info.InsertText = info.Label
			}

			keywords = append(keywords, &info)
		}
	}

	return keywords
}
