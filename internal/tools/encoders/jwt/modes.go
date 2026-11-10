package jwt

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/ui"
)

// jwtTool mirrors the desktop tool's encode/decode configuration while
// retaining independent editor contents when switching modes.
type jwtTool struct {
	*tool
	header							toolkit.SettingHeader
	modeHeaderHeight					int
	modeCompact						bool	// 极矮终端：设置值内联进模式行，省出一行内容区
	encoder							*toolkit.ThreePane
	claimsForm						*toolkit.ThreePane
	encodeForm						*encodeClaimForm
	encClaims						encoderClaims
	encodeConfiguring					bool
	configuring						bool
	key							editor.Model
	encoding, base64Key, keyFocused				bool
	algorithm						int
	encodeRevision						uint64
	encodeRunning, encodeQueued				bool
	width, height, headerHeight, keyHeight, encoderHeight	int
}

func newJWTTool() *jwtTool {
	key := editor.New()
	key.Placeholder = "Enter a secret or PEM private key (Ctrl+S)"
	key.ShowLineNumbers = false
	p := toolkit.NewThreePane("Header", "Payload")
	p.Titles[2] = "Token"
	p.SetOutputFirst(true)
	p.SetExtendable(0, 1)
	p.SetExpandKey("ctrl+e")
	p.A.SetValue(`{"typ":"JWT"}`)
	p.B.SetValue(`{}`)
	// The C# encoder places the generated token before the header and payload.
	// Keep that first row selected when a short terminal can only show one pane.
	p.FocusPane(2)
	form := toolkit.NewThreePane("Expected issuers", "Expected audiences")
	form.Titles[2] = "Claim checks"
	form.A.Placeholder = "Comma-separated issuer values"
	form.B.Placeholder = "Comma-separated audience values"
	base := newTool()
	t := &jwtTool{tool: base, claimsForm: form, encodeForm: newEncodeClaimForm(), header: toolkit.SettingHeader{Names: []string{"Mode", "Algorithm", "Key format"}}, encoder: p, key: key, width: 80, height: 24}
	t.tool.desktop = newDecoderDesktop()
	t.resize()
	t.encoder.Err = fmt.Errorf("a signing key is required")
	return t
}
func (t *jwtTool) Title() string	{ return "JWT Encoder / Decoder" }
func (t *jwtTool) ShortTitle() string	{ return "JWT" }
func (t *jwtTool) Description() string	{ return "Encode, decode and verify JSON Web Tokens" }
func (t *jwtTool) values() []string {
	if !t.encoding {
		format := "Text key"
		if t.tool.base64Key {
			format = "Base64 key"
		}
		if t.height <= 8 {
			// 极矮终端只保留一个设置，给内容区让出空间。
			return []string{format}
		}
		view := "Configure · Alt+V"
		if t.configuring {
			view = "Back · Alt+V"
		}
		onOff := func(b bool) string {
			if b {
				return "On"
			}
			return "Off"
		}
		return []string{format, view, onOff(t.claims.lifetime), onOff(t.claims.issuer), onOff(t.claims.audience), onOff(t.validateToken), onOff(t.validateSignature)}
	}
	format := "Plain text"
	if t.base64Key {
		format = "Base64"
	}
	if !strings.HasPrefix(algorithms[t.algorithm], "HS") {
		format = "PEM"
	}
	if t.height <= 8 {
		return []string{algorithms[t.algorithm]}
	}
	view := "Configure · Alt+V"
	if t.encodeConfiguring {
		view = "Back · Alt+V"
	}
	if t.height <= 12 {
		// 中等高度收起声明开关，保证编码器与密钥编辑器都可见。
		return []string{algorithms[t.algorithm], format, view}
	}
	values := []string{algorithms[t.algorithm], format, view}
	state := func(label string, on bool) string {
		if on {
			return label + ": On"
		}
		return label + ": Off"
	}
	values = append(values, state("Issuer", t.encClaims.issuerEnabled), state("Audience", t.encClaims.audienceEnabled), state("Expiration", t.encClaims.expirationEnabled), state("Default time", t.encClaims.defaultTimes))
	return values
}
func (t *jwtTool) resize() tea.Cmd {
	if t.encoding {
		t.header.Names = []string{"Token hashing algorithm", "Key format", "Claim values", "Token has issuer", "Token has audience", "Token has expiration", "Default times"}
		t.header.Icons = []string{"󰌆", "󰌆", "󰒓", "󰗊", "󰗊", "󰔚", "󰔚"}
		t.header.Descriptions = []string{"Select token parameters", "", "", "", "", "", ""}
		t.header.Groups = []toolkit.SettingGroup{{Title: "Settings", Icon: "󰒓", Primary: 0, Children: []int{3, 4, 5, 6, 1, 2}}}
	} else {
		t.header.Names = []string{"Signature format", "Claim values", "Validate lifetime", "Validate issuer", "Validate audience", "Token validation settings", "Validate issuer signing key"}
		t.header.Icons = []string{"󰌆", "󰒓", "󰔚", "󰗊", "󰗊", "󰒓", "󰌆"}
		t.header.Descriptions = []string{"Plain text / Base64", "", "", "", "", "Select which token parameters to validate", ""}
		t.header.Groups = []toolkit.SettingGroup{{Title: "Token validation settings", Icon: "󰒓", Primary: 5, Children: []int{6, 0, 3, 4, 2, 1}}}
	}
	t.header.HideTitle = true
	t.modeHeaderHeight = 3
	if t.height <= 8 {
		t.modeHeaderHeight = 1
	}
	t.modeCompact = t.height <= 7
	if t.modeCompact {
		// 极矮终端：模式行内联全部设置值，不再单独渲染设置条。
		t.headerHeight = 1
	} else {
		contentGap := t.modeContentGap()
		t.header.SetSize(tea.WindowSizeMsg{Width: t.width, Height: max(1, t.height-t.modeHeaderHeight-contentGap)}, t.values()...)
		// SettingHeader 的紧凑回退按钮条由 ActionHeader 共享，只有指针方法
		// （Index/SetSize）会刷新按钮；值变化后（如切换模式）需触发一次
		// 无副作用的命中查询来刷新缓存，否则 View 渲染滞留旧标签。
		t.header.Index(tea.MouseMsg{Action: tea.MouseActionRelease}, t.values()...)
		settingsHeight := t.header.Height(t.values()...)
		t.headerHeight = t.modeHeaderHeight + contentGap + settingsHeight
	}
	remaining := max(1, t.height-t.headerHeight)
	if !t.encoding {
		if t.configuring {
			t.claimsForm.SetSize(t.width, remaining)
			return nil
		}
		_, cmd := t.tool.Update(tea.WindowSizeMsg{Width: t.width, Height: remaining})
		return cmd
	}
	if t.encodeConfiguring {
		t.encodeForm.resize(t.width, remaining)
		return nil
	}
	// 签名密钥编辑器按剩余高度收缩；极矮终端直接隐藏（Ctrl+S 仍可输入）。
	t.keyHeight = 5
	if remaining < 12 {
		t.keyHeight = 4
	}
	if remaining < 9 {
		t.keyHeight = 1
	}
	if remaining < 6 {
		t.keyHeight = 0
	}
	t.key.SetHeight(max(1, t.keyHeight-2))
	t.key.SetWidth(max(1, t.width))
	t.encoderHeight = max(1, remaining-t.keyHeight)
	t.encoder.SetSize(t.width, t.encoderHeight)
	return nil
}

func (t *jwtTool) modeContentGap() int {
	if t.modeHeaderHeight == 1 {
		return 0
	}
	return 1
}

func (t *jwtTool) toggleMode() tea.Cmd {
	t.encoding = !t.encoding
	t.keyFocused = false
	t.key.Blur()
	if t.encoding {
		if t.encodeConfiguring {
			t.encodeForm.setFocus(t.encodeForm.focus)
		} else {
			t.encoder.FocusPane(t.encoder.Focus)
		}
	}
	return t.resize()
}

type encodeDue struct{ revision uint64 }
type encodeDone struct {
	revision	uint64
	token		string
	err		error
}

func (t *jwtTool) scheduleEncoder() tea.Cmd {
	t.encodeRevision++
	t.encodeQueued = false
	t.encoder.Err = nil
	t.encoder.Copied = false
	t.encoder.SetOutput("")
	t.encoder.Output.SetContent("Encoding…")
	rev := t.encodeRevision
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: encodeDue{revision: rev}} })
}
func (t *jwtTool) startEncoder(rev uint64) tea.Cmd {
	t.encodeRunning = true
	header, payload, key := t.encoder.A.Value(), t.encoder.B.Value(), t.key.Value()
	algorithm, base64Key := algorithms[t.algorithm], t.base64Key
	options, optionsErr := t.encodeForm.options(t.encClaims)
	return func() tea.Msg {
		var token string
		err := optionsErr
		if err == nil {
			token, err = signTokenWithClaims(header, payload, algorithm, key, base64Key, options, time.Now())
		}
		return toolkit.BackgroundToolMsg{Tool: t, Message: encodeDone{revision: rev, token: token, err: err}}
	}
}
func (t *jwtTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case encodeDue:
		if event.revision != t.encodeRevision {
			return t, nil
		}
		if t.encodeRunning {
			t.encodeQueued = true
			return t, nil
		}
		return t, t.startEncoder(event.revision)
	case encodeDone:
		t.encodeRunning = false
		if event.revision == t.encodeRevision {
			t.encoder.Err = event.err
			t.encoder.SetOutput(event.token)
		}
		if t.encodeQueued {
			t.encodeQueued = false
			return t, t.startEncoder(t.encodeRevision)
		}
		return t, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		t.width, t.height = size.Width, size.Height
		return t, t.resize()
	}
	headerMsg := msg
	if mouse, ok := msg.(tea.MouseMsg); ok {
		// 模式行是唯一可切换 Encode/Decode 的行：紧凑布局与单行模式头
		// 渲染在第 0 行，标准三行模式头渲染在第 1 行。
		modeRow := 1
		if t.modeCompact || t.modeHeaderHeight == 1 {
			modeRow = 0
		}
		if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y == modeRow {
			return t, t.toggleMode()
		}
		mouse.Y -= t.modeHeaderHeight + t.modeContentGap()
		headerMsg = mouse
	}
	t.syncSettings()
	setting, choice, handled := t.header.Pick(headerMsg, t.values()...)
	if handled && setting < 0 {
		// 下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if handled && setting == 1 && choice >= 0 && t.encoding {
		// 编码模式的密钥格式下拉框（仅 HS 算法）：选中即设置明文/Base64。
		t.base64Key = choice == 1
		return t, t.scheduleEncoder()
	}
	if handled && setting == 0 && choice >= 0 {
		// 第 0 项的下拉框选中：编码模式选算法，解码模式选密钥格式。
		if t.encoding {
			t.algorithm = choice
			return t, tea.Batch(t.resize(), t.scheduleEncoder())
		}
		t.tool.base64Key = choice == 1
		t.tool.recalc()
		return t, t.resize()
	}
	option := -1
	if handled {
		option = setting + 1
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+t":
			option = 0
		case "alt+a":
			if t.encoding {
				option = 1
			} else {
				option = 5
			}
		case "alt+v":
			if t.encoding {
				option = 3
			} else {
				option = 2
			}
		case "alt+l":
			if !t.encoding {
				option = 3
			}
		case "alt+i":
			if t.encoding {
				option = 4
			} else {
				option = 4
			}
		case "alt+u":
			if t.encoding {
				option = 5
			}
		case "alt+e":
			if t.encoding {
				option = 6
			}
		case "alt+t":
			if t.encoding {
				option = 7
			}
		case "alt+k":
			if t.encoding {
				option = 2
			} else {
				option = 1
			}
		case "ctrl+s":
			if t.encoding && !t.encodeConfiguring {
				t.keyFocused = true
				t.encoder.A.Blur()
				t.encoder.B.Blur()
				return t, t.key.FocusPreservingScroll()
			}
		}
	}
	switch option {
	case 0:
		return t, t.toggleMode()
	case 1:
		if !t.encoding {
			t.tool.base64Key = !t.tool.base64Key
			t.tool.recalc()
			return t, t.resize()
		}
		t.algorithm = (t.algorithm + 1) % len(algorithms)
		return t, tea.Batch(t.resize(), t.scheduleEncoder())
	case 2:
		if !t.encoding {
			t.configuring = !t.configuring
			t.updateClaims()
			return t, t.resize()
		}
		if strings.HasPrefix(algorithms[t.algorithm], "HS") {
			t.base64Key = !t.base64Key
			return t, t.scheduleEncoder()
		}
		return t, nil
	case 3, 4, 5, 6, 7:
		if t.encoding {
			switch option {
			case 3:
				t.encodeConfiguring = !t.encodeConfiguring
				if t.encodeConfiguring {
					t.key.Blur()
					t.encoder.A.Blur()
					t.encoder.B.Blur()
					t.encodeForm.setFocus(t.encodeForm.focus)
				} else if t.keyFocused {
					t.key.FocusPreservingScroll()
				} else {
					t.encoder.FocusPane(t.encoder.Focus)
				}
			case 4:
				t.encClaims.issuerEnabled = !t.encClaims.issuerEnabled
				if t.encClaims.issuerEnabled {
					t.enterEncodeClaims(encodeIssuer)
				}
			case 5:
				t.encClaims.audienceEnabled = !t.encClaims.audienceEnabled
				if t.encClaims.audienceEnabled {
					t.enterEncodeClaims(encodeAudience)
				}
			case 6:
				t.encClaims.expirationEnabled = !t.encClaims.expirationEnabled
				if t.encClaims.expirationEnabled {
					t.enterEncodeClaims(encodeYear)
				}
			case 7:
				t.encClaims.defaultTimes = !t.encClaims.defaultTimes
			}
			if option != 3 {
				return t, tea.Batch(t.resize(), t.scheduleEncoder())
			}
			return t, t.resize()
		}
		if !t.encoding {
			switch option {
			case 3:
				t.claims.lifetime = !t.claims.lifetime
			case 4:
				t.claims.issuer = !t.claims.issuer
			case 5:
				t.claims.audience = !t.claims.audience
			case 6:
				t.validateToken = !t.validateToken
			case 7:
				t.validateSignature = !t.validateSignature
			}
			t.updateClaims()
			return t, t.resize()
		}
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < t.headerHeight {
			return t, nil
		}
		mouse.Y -= t.headerHeight
		if !t.encoding {
			if t.configuring {
				return t, t.updateClaimForm(mouse)
			}
			_, cmd := t.tool.Update(mouse)
			return t, cmd
		}
		if t.encodeConfiguring {
			changed, cmd := t.encodeForm.Update(mouse)
			if changed {
				cmd = tea.Batch(cmd, t.scheduleEncoder())
			}
			return t, cmd
		}
		if mouse.Y >= t.encoderHeight {
			mouse.Y -= t.encoderHeight
			if mouse.Y < t.keyHeight && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
				t.keyFocused = true
				t.encoder.A.Blur()
				t.encoder.B.Blur()
				t.key.FocusPreservingScroll()
				if mouse.Y > 0 {
					return t, t.key.PlaceCursor(mouse.X, mouse.Y-1)
				}
			}
			return t, nil
		}
		if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
			t.keyFocused = false
			t.key.Blur()
		}
		beforeA, beforeB := t.encoder.A.Value(), t.encoder.B.Value()
		cmd := t.encoder.Update(mouse)
		if beforeA != t.encoder.A.Value() || beforeB != t.encoder.B.Value() {
			cmd = tea.Batch(cmd, t.scheduleEncoder())
		}
		return t, cmd
	}
	if !t.encoding {
		if t.configuring {
			return t, t.updateClaimForm(msg)
		}
		_, cmd := t.tool.Update(msg)
		return t, cmd
	}
	if t.encodeConfiguring {
		changed, cmd := t.encodeForm.Update(msg)
		if changed {
			cmd = tea.Batch(cmd, t.scheduleEncoder())
		}
		return t, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && t.keyFocused && key.String() == "ctrl+y" {
		return t, t.encoder.Update(key)
	}
	if key, ok := msg.(tea.KeyMsg); ok && t.keyFocused && (key.String() == "tab" || key.String() == "shift+tab") {
		t.keyFocused = false
		t.key.Blur()
		return t, t.encoder.Update(key)
	}
	if file, ok := msg.(toolkit.FileImportMsg); ok && t.keyFocused {
		t.encoder.Err = toolkit.InsertFileText(&t.key, file.Text)
		if t.encoder.Err == nil {
			return t, t.scheduleEncoder()
		}
		return t, nil
	}
	if t.keyFocused {
		before := t.key.Value()
		var cmd tea.Cmd
		t.key, cmd = t.key.Update(msg)
		if before != t.key.Value() {
			cmd = tea.Batch(cmd, t.scheduleEncoder())
		}
		return t, cmd
	}
	beforeA, beforeB := t.encoder.A.Value(), t.encoder.B.Value()
	cmd := t.encoder.Update(msg)
	if beforeA != t.encoder.A.Value() || beforeB != t.encoder.B.Value() {
		cmd = tea.Batch(cmd, t.scheduleEncoder())
	}
	return t, cmd
}
func (t *jwtTool) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	modeLabel := "󰒓 Tool Mode"
	// 模式是可点击的两态控件：用 ui.Switch 药丸渲染（Encode 为 on）。
	modeControl := ui.Switch{On: t.encoding, OnLabel: "Encode", OffLabel: "Decode"}.View()
	modeValue := modeControl
	modeRow := ui.FormRowRight(modeLabel, lipgloss.Width(modeLabel), modeValue, t.width)
	configuration := theme.Eyebrow("Configuration") + "\n" + modeRow + "\n" +
		labelStyle.Render(ansi.Truncate("Select which mode you want to use", max(1, t.width), "…"))
	if t.modeCompact {
		configuration = labelStyle.Render("󰒓 Mode") + "  " + modeValue +
			"  " + labelStyle.Render(strings.Join(t.values(), "  "))
		configuration = ansi.Truncate(configuration, max(1, t.width), "")
		return configuration + "\n" + t.restOfView()
	}
	if t.modeHeaderHeight == 1 {
		configuration = labelStyle.Render("󰒓 Mode") + "  " + modeValue
	}
	separator := "\n"
	if t.modeContentGap() > 0 {
		separator = "\n\n"
	}
	t.syncSettings()
	view := configuration + separator + t.header.View(t.values()...) + "\n" + t.restOfView()
	// 设置条位于模式行与分隔行之下；下拉浮层据此定位。
	return t.header.OverlayAt(view, strings.Count(configuration+separator, "\n"))
}

// syncSettings 按当前模式刷新设置条的下拉选项与动作按钮（两者的下标随模式变化）。
func (t *jwtTool) syncSettings() {
	t.header.Options = t.settingOptions()
	t.header.Actions = nil
	t.header.ReadOnly = nil
	switch {
	case t.height <= 8:
		// 极矮终端只有第 0 项。
	case t.encoding:
		// 编码模式：算法、密钥格式、Configure 按钮；非 HS 算法的密钥格式是 PEM，只读展示。
		t.header.Actions = []bool{false, false, true}
		if !strings.HasPrefix(algorithms[t.algorithm], "HS") {
			t.header.ReadOnly = []bool{false, true}
		}
	default:
		// 解码模式：密钥格式、Configure 按钮。
		t.header.Actions = []bool{false, true}
	}
}

// settingOptions 返回设置条第 0 项的下拉选项（随模式变化）：
// 编码模式是签名算法（HS/RS/PS/ES 共 12 种），解码模式是密钥格式（文本/Base64）。
func (t *jwtTool) settingOptions() [][]string {
	if t.encoding {
		if strings.HasPrefix(algorithms[t.algorithm], "HS") {
			return [][]string{algorithms, {"Plain text", "Base64"}}
		}
		return [][]string{algorithms}
	}
	return [][]string{{"Text key", "Base64 key"}}
}

// HandlesEscape 下拉浮层展开时 esc 只收起浮层，不退出工具。
func (t *jwtTool) HandlesEscape() bool	{ return t.header.MenuOpen() }

// restOfView 渲染模式行/设置条下方的内容区（解码器、声明表单或编码器）。
func (t *jwtTool) restOfView() string {
	if !t.encoding {
		if t.configuring {
			return t.claimsForm.View()
		}
		return t.tool.View()
	}
	if t.encodeConfiguring {
		return t.encodeForm.View(t.encoder.Err)
	}
	label := "Signing key · Ctrl+S"
	if strings.HasPrefix(algorithms[t.algorithm], "RS") || strings.HasPrefix(algorithms[t.algorithm], "PS") || strings.HasPrefix(algorithms[t.algorithm], "ES") {
		label = "PEM private key · Ctrl+S"
	}
	label = secretLabelStyle.Render(label)
	if t.keyHeight <= 0 {
		return t.encoder.View()
	}
	var key string
	if t.keyHeight == 1 {
		if t.keyFocused {
			key = ansi.Truncate(t.key.View(), t.width, "")
		} else {
			key = ansi.Truncate(label, t.width, "")
		}
	} else {
		signature := ""
		if parts := strings.Split(t.encoder.OutputText(), "."); len(parts) == 3 {
			signature = parts[2]
		}
		signatureRow := secretLabelStyle.Render("Signature") + "  " + ansi.Truncate(signature, max(1, t.width-lipgloss.Width("Signature  ")), "…")
		key = signatureRow + "\n" + ansi.Truncate(label, t.width, "") + "\n" + t.key.View()
	}
	return t.encoder.View() + "\n" + lipgloss.NewStyle().Height(t.keyHeight).Render(key)
}

func (t *jwtTool) updateClaims() {
	t.claims.issuers = t.claimsForm.A.Value()
	t.claims.audiences = t.claimsForm.B.Value()
	t.tool.recalc()
	token := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, t.tool.Input.Value())
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.claimsForm.SetOutput("Enter a token to see claim checks.\nSeparate expected values with commas.\nAlt+L lifetime · Alt+I issuer · Alt+A audience · Alt+V back")
		return
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		t.claimsForm.SetOutput("Invalid token payload.")
		return
	}
	t.claimsForm.SetOutput(checkClaims(payload, t.claims, time.Now()) + "\n\nAlt+L lifetime · Alt+I issuer · Alt+A audience · Alt+V back")
}

func (t *jwtTool) updateClaimForm(msg tea.Msg) tea.Cmd {
	beforeA, beforeB := t.claimsForm.A.Value(), t.claimsForm.B.Value()
	cmd := t.claimsForm.Update(msg)
	if beforeA != t.claimsForm.A.Value() || beforeB != t.claimsForm.B.Value() {
		t.updateClaims()
	}
	return cmd
}

func (t *jwtTool) enterEncodeClaims(f encodeField) {
	t.encodeConfiguring = true
	t.key.Blur()
	t.encoder.A.Blur()
	t.encoder.B.Blur()
	t.encodeForm.setFocus(f)
}
