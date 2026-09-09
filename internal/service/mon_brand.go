package service

import "regexp"

// 机型代号 → 品牌启发式映射（展示/告警摘要/webhook 用，见 PLAN-frontend-monitor.md）。
// Android deviceModel 为产品代号（PKL110/2211133C/HBN-AL00），映射成品牌便于人读；
// 输出为独立 device_brand 字段，不改写原始代号（模糊查询仍按库值匹配）。
// 华为/荣耀共用 XXX-AN00 格式无法细分，合并展示。新增代号模式往 monBrandRules 加一条即可。
var monBrandRules = []struct {
	re    *regexp.Regexp
	brand string
}{
	{regexp.MustCompile(`(?i)^(HUAWEI|HONOR|HW-)`), "华为"},
	{regexp.MustCompile(`^[A-Z]{3}-[A-Z0-9]{4,}$`), "华为/荣耀"}, // HBN-AL00 / ELS-AN00（共用格式）
	{regexp.MustCompile(`^SM-`), "三星"},
	{regexp.MustCompile(`^V\d{4}[A-Z]$`), "vivo"}, // V2302A
	{regexp.MustCompile(`(?i)^iQOO`), "iQOO"},
	{regexp.MustCompile(`(?i)^(P[A-Z]{2}\d{3,}|OPPO|CPH)`), "OPPO"}, // PKL110 / PHU110
	{regexp.MustCompile(`^RMX`), "realme"},
	{regexp.MustCompile(`^\d{4,}[A-Z0-9]*$`), "小米/Redmi"}, // 2211133C / 24031PN0DC
	{regexp.MustCompile(`(?i)^(Redmi|MI\s|MI-|M20\d\d|21\d{3})`), "小米/Redmi"},
	{regexp.MustCompile(`(?i)^(LE\d{3,}|GM5000|OnePlus)`), "一加"},
	{regexp.MustCompile(`(?i)^Pixel`), "谷歌"},
	{regexp.MustCompile(`(?i)^(iPhone|iPad)`), "苹果"},
	{regexp.MustCompile(`(?i)^PC$`), "PC"}, // H5 端 PC 浏览器
}

// monDeviceBrand 代号 → 品牌；未匹配返回空串（调用方原样展示代号）
func monDeviceBrand(model string) string {
	if model == "" {
		return ""
	}
	for _, r := range monBrandRules {
		if r.re.MatchString(model) {
			return r.brand
		}
	}
	return ""
}
