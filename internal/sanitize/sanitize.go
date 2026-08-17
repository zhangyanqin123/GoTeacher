// Package sanitize 富文本 HTML 白名单净化（存储型 XSS 主防线）。
// 独立成包而非挂在 service 下：C 端写路径将来复用同一白名单，保证两端净化策略一致。
package sanitize

import (
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

var (
	once   sync.Once
	policy *bluemonday.Policy
)

// RichText 净化富文本 HTML，返回白名单内的安全子集。
// 空串直接返回；对自身输出幂等（读路径对已净化数据二次净化无畸变，单测固化）。
func RichText(s string) string {
	if s == "" {
		return ""
	}
	once.Do(func() { policy = buildPolicy() })
	return policy.Sanitize(s)
}

// buildPolicy UGC 白名单策略：
//   - 基线 UGCPolicy：a[href]+AllowStandardURLs、img[src/alt/width/height]、
//     p/br/strong/em/ul/ol/li/blockquote/pre/code/h1-h6 等，链接自动加 rel="nofollow"
//   - 补齐编辑器常用结构元素：div/span/table 系/hr/sub/sup/del/ins
//   - style 受限保留：TinyMCE 颜色/字号工具栏输出内联样式，全剥离损失产品能力；
//     bluemonday 的 CSS 解析器只放行声明好的属性名，且值中 url()/expression()/函数
//     一律丢弃，风险收敛为视觉样式滥用，无脚本执行面。属性集按元素并集声明
//     （v1.0.27 API 是全局属性集 × 元素列表，不支持按元素差异化属性）
//   - 不放行（默认即无）：script/iframe/object/embed/form/style 标签、link/meta/base、
//     所有 on* 事件属性
func buildPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowElements("div", "span", "hr", "sub", "sup", "del", "ins")
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th")
	p.AllowAttrs("colspan", "rowspan").Matching(bluemonday.Integer).OnElements("td", "th")

	p.AllowStyles(
		"color", "background-color", "font-size", "font-weight",
		"font-style", "text-align", "text-decoration",
	).OnElements("p", "span", "div", "li", "td", "th", "h1", "h2", "h3", "h4", "h5", "h6")

	return p
}
