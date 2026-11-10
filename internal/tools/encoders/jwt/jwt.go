// Package jwt 提供 JWT 解码工具：
// 分色显示 Header/Payload 的 JSON，标注 exp/iat/nbf 时间字段的
// 含义与本地时间；可选填 HS256 密钥进行验签。
package jwt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toy-forge/devtoys/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// focus 表示 JWT 工具中当前持有焦点的区域：令牌 / 密钥 / 输出。
type focus int

const (
	focusToken	focus	= iota	// 令牌输入（Workspace 的输入区）
	focusSecret			// HS256 密钥输入
	focusOutput			// 输出区（可滚动）
)

// timeFieldDesc 说明各时间字段的含义。
var timeFieldDesc = map[string]string{
	"exp":	"Expiration time",
	"iat":	"Issued at",
	"nbf":	"Not valid before",
}

var (
	sectionStyle	= lipgloss.NewStyle().Foreground(theme.Blue).Bold(true)	// 段落标题
	jsonKeyStyle	= lipgloss.NewStyle().Foreground(theme.BlueBright)	// JSON 键
	jsonStrStyle	= lipgloss.NewStyle().Foreground(theme.Green)		// JSON 字符串
	jsonNumStyle	= lipgloss.NewStyle().Foreground(theme.YellowBright)	// JSON 数字
	jsonBoolStyle	= lipgloss.NewStyle().Foreground(theme.Magenta)		// JSON 布尔
	jsonNullStyle	= lipgloss.NewStyle().Foreground(theme.Muted)		// JSON null

	okStyle		= lipgloss.NewStyle().Foreground(theme.Green).Bold(true)
	badStyle	= lipgloss.NewStyle().Foreground(theme.Red).Bold(true)
	warnStyle	= lipgloss.NewStyle().Foreground(theme.Yellow)
	timeStyle	= lipgloss.NewStyle().Foreground(theme.Text)
	staleStyle	= lipgloss.NewStyle().Foreground(theme.Red)
	freshStyle	= lipgloss.NewStyle().Foreground(theme.Green)

	secretLabelStyle	= lipgloss.NewStyle().Foreground(theme.Muted)
	// 面板内容 1 列内边距：与 ui.Panel 语言一致。
	secretPaneStyle	= lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(0, ui.Space1)
	secretPaneFocusedStyle	= lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.Accent).
				Padding(0, ui.Space1)
)

// tool 是 JWT 解码工具模型：在公共双栏工作区之上增加一行密钥输入。
type tool struct {
	toolkit.Workspace

	secret					textarea.Model	// HS256 密钥（可选）
	headerHeight				int
	width, height				int
	cur					focus	// 当前焦点
	base64Key				bool
	validateToken, validateSignature	bool
	claims					claimOptions
	desktop					*decoderDesktop
}

// newTool 创建工具实例并在 init 中注册。
func newTool() *tool {
	sec := textarea.New()
	sec.Placeholder = "Optional HMAC key or PEM public key"
	sec.ShowLineNumbers = false
	sec.SetHeight(1)
	sec.SetWidth(24)

	t := &tool{
		Workspace:		toolkit.NewWorkspace("Paste JWT (header.payload.signature)…", ""),
		secret:			sec,
		cur:			focusToken,
		headerHeight:		3,
		validateToken:		true,
		validateSignature:	true,
	}
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newJWTTool()) }

func (t *tool) ID() string		{ return "encoders.jwt" }
func (t *tool) Title() string		{ return "JWT Decoder" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰌾" }
func (t *tool) Description() string {
	return "Decode JWT and optionally verify its signature"
}

func (t *tool) Init() tea.Cmd	{ return nil }

// PaneStatusHints 供外壳状态栏合并展示本工具独有的键。
func (t *tool) PaneStatusHints() []ui.StatusHint {
	if t.desktop != nil && t.desktop.active {
		hints := []ui.StatusHint{
			{Key: "click", Desc: "edit"},
			{Key: "scroll", Desc: "browse"},
			{Key: "alt+c", Desc: "claims"},
			{Key: "ctrl+s", Desc: "key"},
		}
		desc := "expand"
		if t.desktop.expanded {
			desc = "restore"
		}
		if t.desktop.expanded || t.desktop.focus == 3 {
			hints = append(hints, ui.StatusHint{Key: "ctrl+e", Desc: desc})
		}
		return hints
	}
	return t.Workspace.StatusHints()
}

func (t *tool) keyTitle() string {
	token := strings.Join(strings.Fields(t.Input.Value()), "")
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		if headerJSON, err := decodeSegment(parts[0]); err == nil {
			var header map[string]any
			if json.Unmarshal(headerJSON, &header) == nil {
				if algorithm, ok := header["alg"].(string); ok {
					if strings.HasPrefix(algorithm, "RS") || strings.HasPrefix(algorithm, "PS") || strings.HasPrefix(algorithm, "ES") {
						return "Public Key"
					}
				}
			}
		}
	}
	return "Verification key"
}

// verificationKeyVisible reports whether the verification key row/pane is shown.
// It follows the "Token validation settings" switch rather than the token's
// algorithm, so the key stays reachable for pasting before a token arrives.
func (t *tool) verificationKeyVisible() bool {
	return t.validateToken && t.validateSignature
}

// Update 处理焦点切换与密钥输入，其余按键按焦点路由。
// tab/shift+tab 在 令牌 → 密钥 → 输出 之间循环（覆盖公共工作区
// 的两栏切换），ctrl+s 直接聚焦密钥，ctrl+y 始终复制输出。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.desktop != nil {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			t.desktop.resize(t, size.Width, size.Height)
		}
		if t.desktop.active {
			return t, t.desktop.update(t, msg)
		}
	}
	if file, ok := msg.(toolkit.FileImportMsg); ok {
		if t.cur == focusSecret {
			if err := toolkit.InsertFileText(&t.secret, file.Text); err != nil {
				t.SetError(err)
				return t, nil
			}
		} else {
			t.cur = focusToken
			t.applyFocus()
			t.Workspace.Update(file)
			if t.Workspace.Err != nil {
				return t, nil
			}
		}
		t.recalc()
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+y":
			return t, t.Workspace.Update(msg)
		case "ctrl+s":
			t.cur = focusSecret
			t.applyFocus()
			return t, t.secret.FocusPreservingScroll()
		case "tab":
			t.cycleFocus(1)
			return t, nil
		case "shift+tab":
			t.cycleFocus(-1)
			return t, nil
		}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		t.width, t.height = sz.Width, sz.Height
		t.headerHeight = 3
		if sz.Height <= 7 {
			t.headerHeight = 2
		}
		if sz.Height <= 6 {
			t.headerHeight = 1
		}
		switch t.headerHeight {
		case 1:
			t.secret.SetWidth(max(2, sz.Width-lipgloss.Width("Secret ")))
		case 2:
			t.secret.SetWidth(max(2, sz.Width))
		default:
			// 密钥行窗格含 1 列/侧内边距：宽度预算同步扣减。
			t.secret.SetWidth(max(2, sz.Width-lipgloss.Width("Verification key ")-2-ui.Space1*2))
		}
		return t, t.Workspace.Update(tea.WindowSizeMsg{Width: sz.Width, Height: max(1, sz.Height-t.headerHeight)})
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		// 密钥行渲染在工作区上方：headerHeight 行内的点击属于密钥区。
		if t.verificationKeyVisible() && mouse.Y < t.headerHeight {
			if mouse.Action == tea.MouseActionPress && (mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown) {
				delta := 3
				if mouse.Button == tea.MouseButtonWheelUp {
					delta = -3
				}
				t.secret.Scroll(delta)
				return t, nil
			}
			if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
				t.cur = focusSecret
				t.applyFocus()
				switch t.headerHeight {
				case 1:
					prefix := lipgloss.Width("Secret ")
					if mouse.X >= prefix {
						return t, t.secret.PlaceCursor(mouse.X-prefix, 0)
					}
				case 2:
					if mouse.Y == 1 {
						return t, t.secret.PlaceCursor(mouse.X, 0)
					}
				default:
					prefix := lipgloss.Width(t.keyTitle() + " ")
					if mouse.Y == 1 && mouse.X > prefix && mouse.X < prefix+lipgloss.Width(secretPaneStyle.Render(t.secret.View()))-1 {
						return t, t.secret.PlaceCursor(mouse.X-prefix-1, 0)
					}
				}
				return t, nil
			}
			return t, nil
		}
		local := mouse
		if t.verificationKeyVisible() {
			local.Y -= t.headerHeight
		}
		if local.Y < 0 {
			return t, nil
		}
		cmd := t.Workspace.UpdateBelow(local, 0)
		if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
			t.cur = focusToken
			if t.Workspace.Focus == toolkit.FocusOutput {
				t.cur = focusOutput
			}
			t.applyFocus()
		}
		return t, cmd
	}

	switch t.cur {
	case focusSecret:
		var cmd tea.Cmd
		t.secret, cmd = t.secret.Update(msg)
		t.recalc()
		return t, cmd
	default:
		cmd := t.Workspace.Update(msg)
		t.recalc()
		return t, cmd
	}
}

// cycleFocus 按 dir（1/-1）在三个焦点区域间循环并应用焦点状态。
func (t *tool) cycleFocus(dir int) {
	next := (int(t.cur) + dir + 3) % 3
	t.cur = focus(next)
	t.applyFocus()
}

// applyFocus 把焦点状态同步到输入框与公共工作区。
func (t *tool) applyFocus() {
	switch t.cur {
	case focusToken:
		t.Input.FocusPreservingScroll()
		t.secret.Blur()
		t.Workspace.Focus = toolkit.FocusInput
	case focusSecret:
		t.Input.Blur()
		t.secret.FocusPreservingScroll()
		// 设为输入焦点可避免按键误触输出区滚动。
		t.Workspace.Focus = toolkit.FocusInput
	case focusOutput:
		t.Input.Blur()
		t.secret.Blur()
		t.Workspace.Focus = toolkit.FocusOutput
	}
}

// View 渲染密钥输入行（工作区上方）+ 双栏工作区。
func (t *tool) View() string {
	if t.desktop != nil && t.desktop.active {
		return t.desktop.view(t)
	}
	var bar string
	switch t.headerHeight {
	case 1:
		bar = lipgloss.JoinHorizontal(lipgloss.Top, secretLabelStyle.Render("Secret "), t.secret.View())
	case 2:
		bar = secretLabelStyle.Render(t.keyTitle()) + "\n" + t.secret.View()
	default:
		label := secretLabelStyle.Render(t.keyTitle() + " ")
		paneStyle := secretPaneStyle
		if t.cur == focusSecret {
			paneStyle = secretPaneFocusedStyle
		}
		bar = lipgloss.JoinHorizontal(lipgloss.Top, label, paneStyle.Render(t.secret.View()))
	}
	view := t.Workspace.View()
	if !t.verificationKeyVisible() {
		return view
	}
	return bar + "\n" + view
}

// recalc 解析当前令牌（与可选密钥），分色渲染结果；错误写入信息条。
func (t *tool) recalc() {
	if t.desktop != nil {
		defer t.desktop.refresh(t)
	}
	token := strings.TrimSpace(t.Input.Value())
	if token == "" {
		t.SetError(nil)
		t.SetOutput("Paste a JWT (header.payload.signature) into the input")
		return
	}

	// JWT 允许前后有空白；去掉中间空白以防复制时带入换行。
	token = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, token)

	out, err := renderTokenWithValidation(token, t.secret.Value(), t.base64Key, t.validateToken, t.validateSignature)
	if err != nil {
		t.SetError(err)
		t.SetOutput("")
		return
	}
	t.SetError(nil)
	if t.validateToken && t.claims.enabled() {
		parts := strings.Split(token, ".")
		payload, decodeErr := decodeSegment(parts[1])
		if decodeErr == nil {
			out += "\n\n" + checkClaims(payload, t.claims, time.Now())
		}
	}
	t.SetOutput(out)
}

// renderToken 解码 JWT 的三段并渲染为分色文本。
func renderToken(token, secret string) (string, error) {
	return renderTokenWithKeyFormat(token, secret, false)
}

func renderTokenWithKeyFormat(token, secret string, base64Key bool) (string, error) {
	return renderTokenWithValidation(token, secret, base64Key, true, true)
}

func renderTokenWithValidation(token, secret string, base64Key, validateToken, validateSignature bool) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("JWT must have 3 segments (header.payload.signature); got %d", len(parts))
	}

	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return "", fmt.Errorf("header decode failed: %w", err)
	}
	payloadJSON, err := decodeSegment(parts[1])
	if err != nil {
		return "", fmt.Errorf("payload decode failed: %w", err)
	}

	var header map[string]any
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", fmt.Errorf("header JSON parse failed: %w", err)
	}
	if header == nil {
		return "", fmt.Errorf("header JSON must be an object")
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", fmt.Errorf("payload JSON parse failed: %w", err)
	}
	if payload == nil {
		return "", fmt.Errorf("payload JSON must be an object")
	}

	var b strings.Builder

	if validateToken {
		// ── 验签结论 ──
		if validateSignature {
			b.WriteString(verifySummary(parts, header, secret, base64Key))
		} else {
			b.WriteString(warnStyle.Render("Signature validation is disabled"))
		}
		b.WriteString("\n\n")
	}

	// ── Header ──
	b.WriteString(sectionStyle.Render("■ HEADER"))
	b.WriteString("\n")
	b.WriteString(colorizeJSON(headerJSON))
	b.WriteString("\n\n")

	// ── Payload ──
	b.WriteString(sectionStyle.Render("■ PAYLOAD"))
	b.WriteString("\n")
	b.WriteString(colorizeJSON(payloadJSON))
	b.WriteString("\n\n")

	// ── 签名 ──
	b.WriteString(sectionStyle.Render("■ SIGNATURE"))
	b.WriteString("\n")
	b.WriteString(timeStyle.Render(parts[2]))
	b.WriteString("\n\n")

	// ── 时间字段标注 ──
	if tf := renderTimeFields(payloadJSON); tf != "" {
		b.WriteString(sectionStyle.Render("■ Time claims"))
		b.WriteString("\n")
		b.WriteString(tf)
	}

	return strings.TrimSuffix(b.String(), "\n"), nil
}

// verifySummary 按是否提供密钥给出验签结论行。
func verifySummary(parts []string, header map[string]any, secret string, base64Key bool) string {
	alg, _ := header["alg"].(string)
	if secret == "" {
		return warnStyle.Render("◐ No secret supplied; signature not verified") +
			timeStyle.Render(fmt.Sprintf("（alg=%s）", alg))
	}
	if _, err := signingMethod(alg); err != nil {
		return warnStyle.Render(err.Error())
	}
	if err := verifySignature(parts, alg, secret, base64Key); err != nil {
		return badStyle.Render("Signature verification failed: " + err.Error())
	}
	return okStyle.Render("Signature verified (" + alg + ")")
}

// decodeSegment 解码 JWT 的一段 Base64URL（宽容处理 padding）。
func decodeSegment(seg string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(seg); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("invalid Base64URL")
}

// renderTimeFields 从 payload 中提取 exp/iat/nbf 并标注含义与本地时间。
func renderTimeFields(payloadJSON []byte) string {
	// 用 UseNumber 保留数字字面量，避免 float64 精度丢失。
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(payloadJSON))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return ""
	}

	var b strings.Builder
	now := time.Now()
	for _, name := range []string{"exp", "iat", "nbf"} {
		num, ok := payload[name].(json.Number)
		if !ok {
			continue
		}
		ts, err := num.Float64()
		if err != nil {
			continue
		}
		local := time.Unix(int64(ts), 0)
		line := fmt.Sprintf("%s · %s: %s → %s",
			jsonKeyStyle.Render(name),
			timeFieldDesc[name],
			jsonNumStyle.Render(num.String()),
			timeStyle.Render(local.Format("2006-01-02 15:04:05 MST")),
		)
		// exp/nbf 可以直接给出令牌状态结论。
		switch name {
		case "exp":
			if now.After(local) {
				line += "  " + staleStyle.Render("Expired")
			} else {
				line += "  " + freshStyle.Render("Not expired")
			}
		case "nbf":
			if now.Before(local) {
				line += "  " + staleStyle.Render("Not yet valid")
			} else {
				line += "  " + freshStyle.Render("Valid")
			}
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// colorizeJSON 把 JSON 文本按键/字符串/数字/布尔/null 分色渲染。
// 先统一格式化为两空格缩进，再按行着色，避免手写正则误判嵌套结构。
func colorizeJSON(src []byte) string {
	var v any
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		// 解析失败时原样返回，交由上层错误提示兜底。
		return string(src)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(src)
	}

	var b strings.Builder
	for _, line := range strings.Split(string(pretty), "\n") {
		b.WriteString(colorizeLine(line))
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// colorizeLine 给 JSON 的单行着色：区分 "键": 值 两种角色。
func colorizeLine(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]

	// 形如 "key": value 的行：键按 JSON 键色，值按类型着色。
	if strings.HasPrefix(trimmed, "\"") {
		if i := strings.Index(trimmed, "\":"); i >= 0 {
			key := trimmed[:i+1]
			rest := strings.TrimLeft(trimmed[i+2:], " ")	// 跳过 ":"
			value, comma := splitTrailingComma(rest)
			return indent + jsonKeyStyle.Render(key) + timeStyle.Render(":") +
				" " + colorizeValue(value) + timeStyle.Render(comma)
		}
	}

	value, comma := splitTrailingComma(trimmed)
	return indent + colorizeValue(value) + timeStyle.Render(comma)
}

// splitTrailingComma 把行尾逗号与值拆开（逗号单独按中性色渲染）。
func splitTrailingComma(s string) (value, comma string) {
	if strings.HasSuffix(s, ",") {
		return strings.TrimRight(s[:len(s)-1], " "), ","
	}
	return s, ""
}

// colorizeValue 按字面量类型给 JSON 值着色；括号等结构符号用中性色。
func colorizeValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "\"") {
		return jsonStrStyle.Render(v)
	}
	switch v {
	case "true", "false":
		return jsonBoolStyle.Render(v)
	case "null":
		return jsonNullStyle.Render(v)
	case "{", "}", "[", "]", "{ }":
		return timeStyle.Render(v)
	}
	if isNumericLiteral(v) {
		return jsonNumStyle.Render(v)
	}
	return timeStyle.Render(v)
}

// isNumericLiteral 判断字符串是否为 JSON 数字字面量。
func isNumericLiteral(s string) bool {
	var n json.Number
	return json.Unmarshal([]byte(s), &n) == nil
}
