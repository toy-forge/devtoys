package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// b64url 以 Raw URL-safe Base64 编码字节。
func b64url(b []byte) string	{ return base64.RawURLEncoding.EncodeToString(b) }

// hs256 计算 HMAC-SHA256 签名。
func hs256(data, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// makeToken 构造一个 alg=HS256 的测试令牌，exp 设为 2100 年（未过期）。
func makeToken(secret string) string {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := b64url([]byte(`{"sub":"123","name":"测试","exp":4102444800}`))
	sig := b64url(hs256(header+"."+payload, secret))
	return header + "." + payload + "." + sig
}

// TestDecodeValidToken 验证有效令牌的解码输出包含各段落与时间标注。
func TestDecodeValidToken(t *testing.T) {
	token := makeToken("secret")
	out, err := renderToken(token, "")
	if err != nil {
		t.Fatalf("解码不应报错: %v", err)
	}
	for _, want := range []string{
		"No secret supplied",
		"HEADER",
		"PAYLOAD",
		"SIGNATURE",
		`"alg"`,
		"测试",
		"Time claims",
		"Expiration time",
		"Not expired",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q:\n%s", want, out)
		}
	}
}

// TestVerifySignature 提供正确密钥应验签通过，错误密钥应失败。
func TestVerifySignature(t *testing.T) {
	token := makeToken("secret")

	out, err := renderToken(token, "secret")
	if err != nil {
		t.Fatalf("验签不应报错: %v", err)
	}
	if !strings.Contains(out, "Signature verified") {
		t.Fatalf("正确密钥应验签通过:\n%s", out)
	}

	out, err = renderToken(token, "wrong")
	if err != nil {
		t.Fatalf("错误密钥不应导致解析报错: %v", err)
	}
	if !strings.Contains(out, "Signature verification failed") {
		t.Fatalf("错误密钥应验签失败:\n%s", out)
	}
}

// TestNonHS256 算法不是 HS256 时应提示无法验证。
func TestNonHS256(t *testing.T) {
	header := b64url([]byte(`{"alg":"RS256"}`))
	payload := b64url([]byte(`{"sub":"1"}`))
	token := header + "." + payload + ".c2ln"
	out, err := renderToken(token, "secret")
	if err != nil {
		t.Fatalf("解析不应报错: %v", err)
	}
	if !strings.Contains(out, "invalid verification key") {
		t.Fatalf("应提示仅支持 HS256:\n%s", out)
	}
}

// TestSegmentCount 错误段数应返回友好错误。
func TestSegmentCount(t *testing.T) {
	_, err := renderToken("a.b", "")
	if err == nil {
		t.Fatal("两段令牌应报错")
	}
	if !strings.Contains(err.Error(), "3 segments") || !strings.Contains(err.Error(), "2") {
		t.Fatalf("错误应说明段数，得到: %v", err)
	}

	_, err = renderToken("a.b.c.d", "")
	if err == nil || !strings.Contains(err.Error(), "got 4") {
		t.Fatalf("四段令牌应报 4 段错误，得到: %v", err)
	}
}

// TestBadSegment 段不是 Base64URL / Header 不是 JSON 时应报错。
func TestBadSegment(t *testing.T) {
	if _, err := renderToken("@@@.###.$$$", ""); err == nil {
		t.Fatal("非法 Base64 段应报错")
	}
	if !strings.Contains(renderErr(t, "!!!.$$$.$$$"), "header decode failed") {
		t.Fatal("Header 解码失败应提示")
	}
	// Header 是合法 Base64 但不是 JSON。
	badJSON := b64url([]byte("not json")) + "." + b64url([]byte("{}")) + ".sig"
	if !strings.Contains(renderErr(t, badJSON), "header JSON parse failed") {
		t.Fatal("Header 非 JSON 应提示")
	}
	// Payload 不是合法 Base64。
	if !strings.Contains(renderErr(t, b64url([]byte("{}"))+".!!!.sig"), "payload decode failed") {
		t.Fatal("Payload 解码失败应提示")
	}
}

// renderErr 渲染令牌并返回错误（无输出断言）。
func renderErr(t *testing.T, token string) string {
	t.Helper()
	_, err := renderToken(token, "")
	if err == nil {
		t.Fatalf("renderToken(%q) 应报错", token)
	}
	return err.Error()
}

// TestDecodeSegment 验证 Base64URL 段解码对 padding 的宽容与非法拒绝。
func TestDecodeSegment(t *testing.T) {
	if b, err := decodeSegment("SGVsbG8"); err != nil || string(b) != "Hello" {
		t.Fatalf("Raw 解码失败: %v %q", err, b)
	}
	if b, err := decodeSegment("SGVsbG8="); err != nil || string(b) != "Hello" {
		t.Fatalf("带 padding 解码失败: %v %q", err, b)
	}
	if _, err := decodeSegment("@@@"); err == nil {
		t.Fatal("非法段应报错")
	}
}

// TestColorizeJSON 验证 JSON 分色输出保留键与值。
// 断言前剥离 ANSI 转义序列：着色依赖 lipgloss 的终端能力检测，
// 在 CLICOLOR_FORCE=1 等环境下输出会带颜色码，测试不应受环境影响。
func TestColorizeJSON(t *testing.T) {
	ansiRe := regexp.MustCompile("\x1b\\[[0-9;:]*[ -/]*[@-~]")
	plain := func(s string) string { return ansiRe.ReplaceAllString(s, "") }

	out := plain(string(colorizeJSON([]byte(`{"a":1,"b":[true,null]}`))))
	for _, want := range []string{`"a"`, "1", `"b"`, "true", "null"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q:\n%s", want, out)
		}
	}
	// 缩进格式化应生效。
	if !strings.Contains(out, "\n  \"a\": 1,") {
		t.Fatalf("应按两空格缩进格式化:\n%s", out)
	}
	// 非法 JSON 原样返回不崩溃。
	if got := colorizeJSON([]byte("{bad")); got != "{bad" {
		t.Fatalf("非法 JSON 应原样返回，得到 %q", got)
	}
}

// TestRecalcViaTool 验证工具模型：空输入提示、粘贴令牌后输出结果。
func TestRecalcViaTool(t *testing.T) {
	tl := newTool()
	if err := tl.Err; err != nil {
		t.Fatalf("初始不应有错误: %v", err)
	}
	tl.Input.SetValue(makeToken("secret") + "\n")	// 末尾换行应被剥离
	tl.recalc()
	if tl.Err != nil {
		t.Fatalf("有效令牌不应报错: %v", tl.Err)
	}
	if !strings.Contains(tl.OutputText(), "PAYLOAD") {
		t.Fatalf("输出应包含解析结果:\n%s", tl.OutputText())
	}

	tl.Input.SetValue("only.one")
	tl.recalc()
	if tl.Err == nil || !strings.Contains(tl.Err.Error(), "3 segments") {
		t.Fatalf("非法令牌应报段数错误: %v", tl.Err)
	}
}

func TestMouseAndResizeWhileSecretFocused(t *testing.T) {
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 68, Height: 29})
	tool.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if tool.cur != focusSecret {
		t.Fatal("secret was not focused")
	}
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 35})
	// 底部条上方保留一行空白（toolkit workspaceFooterGap）。
	if tool.Output.Height != 28 {
		t.Fatalf("output height = %d after resize", tool.Output.Height)
	}
	tool.Update(tea.MouseMsg{X: 60, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if tool.cur != focusOutput {
		t.Fatal("output was not focused")
	}
}

func TestSecretClickPositionsCursor(t *testing.T) {
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	tool.secret.SetValue("abcd")
	prefix := lipgloss.Width("Verification key ")
	tool.Update(tea.MouseMsg{X: prefix + 4, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := tool.secret.Value(); got != "aXbcd" {
		t.Fatalf("secret=%q", got)
	}
	if got := lipgloss.Width(tool.View()); got > 30 {
		t.Fatalf("view width=%d", got)
	}
}

func TestShortScreenKeepsSecretAndWorkspaceReachable(t *testing.T) {
	for _, height := range []int{6, 7} {
		tool := newTool()
		tool.Update(tea.WindowSizeMsg{Width: 30, Height: height})
		if got := lipgloss.Height(tool.View()); got != height {
			t.Fatalf("height %d: view height %d", height, got)
		}
		y := tool.headerHeight - 1
		x := 3
		if tool.headerHeight == 1 {
			x = lipgloss.Width("Secret ") + 3
		}
		tool.secret.SetValue("abcd")
		tool.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
		if got := tool.secret.Value(); got != "aXbcd" {
			t.Fatalf("height %d secret=%q", height, got)
		}
	}
}

func TestRejectInvalidJSONObjectSegments(t *testing.T) {
	for _, test := range []struct{ header, payload, want string }{
		{`{}`, `not JSON`, "payload JSON parse failed"},
		{`{}`, `[]`, "payload JSON parse failed"},
		{`{}`, `null`, "payload JSON must be an object"},
		{`null`, `{}`, "header JSON must be an object"},
	} {
		token := b64url([]byte(test.header)) + "." + b64url([]byte(test.payload)) + ".c2ln"
		if _, err := renderToken(token, ""); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s / %s: %v", test.header, test.payload, err)
		}
	}
}
