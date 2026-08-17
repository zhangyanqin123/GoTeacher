package sanitize

import "testing"

// 注入用例 golden 断言：既固化净化行为，也是接口层 curl 注入验证的期望值来源。
// 改 policy 时先跑这里，同步更新 PLAN 文档中的净化矩阵说明。
func TestRichText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串直返", "", ""},
		{"script 标签剥离", "<p>ok<script>alert(1)</script></p>", "<p>ok</p>"},
		{"img 事件属性剥离", `<img src=x onerror=alert(1)>`, `<img src="x">`},
		{"javascript: 协议 href 剥离（连 <a> 一并剥掉）", `<a href="javascript:alert(1)">c</a>`, `c`},
		{"style 只留安全属性", `<span style="color:red;background:url(javascript:1)">t</span>`, `<span style="color: red">t</span>`},
		{"iframe 剥离", "<iframe src=\"//evil\"></iframe>after", "after"},
		{"事件属性统一剥离", `<p onclick="x" onmouseover="y">t</p>`, "<p>t</p>"},
		{"table 结构保留", `<table><tr><td colspan="2">c</td></tr></table>`, `<table><tr><td colspan="2">c</td></tr></table>`},
		{"TinyMCE 样本原样保留", `<p><strong>加粗</strong><span style="color: #e03e3e;">红字</span></p>`, `<p><strong>加粗</strong><span style="color: #e03e3e">红字</span></p>`},
		{"纯文本直返", "hello", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RichText(c.in); got != c.want {
				t.Errorf("RichText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// 幂等性：读路径会对本服务已净化的数据二次净化，必须无畸变。
func TestRichTextIdempotent(t *testing.T) {
	for _, in := range []string{
		"<p>ok<script>alert(1)</script></p>",
		`<span style="color:red;background:url(x)">t</span>`,
		`<img src="x" onerror="alert(1)">`,
		`<a href="javascript:alert(1)">c</a>`,
	} {
		once := RichText(in)
		if twice := RichText(once); twice != once {
			t.Errorf("not idempotent: %q -> %q -> %q", in, once, twice)
		}
	}
}
