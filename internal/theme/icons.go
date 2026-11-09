// icons.go 是集中管理的图标表：Nerd Font 字符 + ASCII 回退。
//
// 目标终端缺少 Nerd Font 时（SetASCII(true) 或 DEVTOYS_ASCII=1），
// Icon 返回等义的 ASCII 字符，避免出现"豆腐块"。
// 行宽一律用 go-runewidth 计算（IconWidth），不要用 len()。
package theme

import (
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// IconName 是图标表中的语义名称。
type IconName string

const (
	IconLogo	IconName	= "logo"	// 应用标识
	IconSearch	IconName	= "search"	// 搜索
	IconInfo	IconName	= "info"	// 信息状态
	IconSuccess	IconName	= "success"	// 成功状态
	IconWarning	IconName	= "warning"	// 警告状态
	IconError	IconName	= "error"	// 错误状态
	IconCopy	IconName	= "copy"	// 复制
	IconPaste	IconName	= "paste"	// 粘贴
	IconFolder	IconName	= "folder"	// 打开文件/目录
	IconClear	IconName	= "clear"	// 清空
	IconExpand	IconName	= "expand"	// 展开窗格
	IconCollapse	IconName	= "collapse"	// 收起窗格
	IconClock	IconName	= "clock"	// 时间类工具
	IconCalendar	IconName	= "calendar"	// 日期类工具
	IconTable	IconName	= "table"	// 表格类工具
	IconTransform	IconName	= "transform"	// 转换类工具
	IconNumber	IconName	= "number"	// 数值类工具
	IconCertificate	IconName	= "certificate"	// 证书类工具
	IconEye		IconName	= "eye"		// 图像/预览类工具
	IconEncode	IconName	= "encode"	// 编码类工具
	IconGlobe	IconName	= "globe"	// 网络/URL 类工具
	IconDatabase	IconName	= "database"	// SQL 类工具
	IconPaint	IconName	= "paint"	// 颜色类工具
	IconFile	IconName	= "file"	// 文本类工具
	IconKey		IconName	= "key"		// 密码/UUID 类工具
	IconWrench	IconName	= "wrench"	// 生成器类工具
	IconBug		IconName	= "bug"		// 校验/测试类工具
	IconBook	IconName	= "book"	// 文档类工具
	IconCompare	IconName	= "compare"	// 对比类工具
	IconZip		IconName	= "zip"		// 压缩类工具
	IconCode	IconName	= "code"	// 代码类工具
)

// nerdIcons 是 Nerd Font 字形表；asciiIcons 是宽度为 1 的 ASCII 回退。
var nerdIcons = map[IconName]string{
	IconLogo:		"󰆍",
	IconSearch:		"󰍉",
	IconInfo:		"󰋼",
	IconSuccess:		"󰄬",
	IconWarning:		"󰀪",
	IconError:		"󰅚",
	IconCopy:		"󰆏",
	IconPaste:		"󰅌",
	IconFolder:		"󰉋",
	IconClear:		"󰗀",
	IconExpand:		"󰊓",
	IconCollapse:		"󰁌",
	IconClock:		"󰥔",
	IconCalendar:		"󰃰",
	IconTable:		"󰓫",
	IconTransform:		"󰘦",
	IconNumber:		"󰎠",
	IconCertificate:	"󰈙",
	IconEye:		"󰋩",
	IconEncode:		"󰌆",
	IconGlobe:		"󰖟",
	IconDatabase:		"󰆼",
	IconPaint:		"󰈈",
	IconFile:		"󰉿",
	IconKey:		"󰘬",
	IconWrench:		"󰒓",
	IconBug:		"󰙨",
	IconBook:		"󰍔",
	IconCompare:		"󰕚",
	IconZip:		"󰀼",
	IconCode:		"󰦝",
}

var asciiIcons = map[IconName]string{
	IconLogo:		"#",
	IconSearch:		"/",
	IconInfo:		"i",
	IconSuccess:		"+",
	IconWarning:		"!",
	IconError:		"x",
	IconCopy:		"c",
	IconPaste:		"p",
	IconFolder:		"F",
	IconClear:		"X",
	IconExpand:		"^",
	IconCollapse:		"v",
	IconClock:		"@",
	IconCalendar:		"D",
	IconTable:		"=",
	IconTransform:		"T",
	IconNumber:		"#",
	IconCertificate:	"C",
	IconEye:		"O",
	IconEncode:		"E",
	IconGlobe:		"*",
	IconDatabase:		"S",
	IconPaint:		"%",
	IconFile:		"T",
	IconKey:		"K",
	IconWrench:		"W",
	IconBug:		"?",
	IconBook:		"B",
	IconCompare:		"<",
	IconZip:		"z",
	IconCode:		"{",
}

// toolIconNames 把工具注册表 ID 映射到语义图标。
var toolIconNames = map[string]IconName{
	"converters.cron":		IconClock,
	"converters.dateparser":	IconCalendar,
	"converters.jsontable":		IconTable,
	"converters.jsonyaml":		IconTransform,
	"converters.numberbase":	IconNumber,
	"encoders.certificate":		IconCertificate,
	"encoders.base64image":		IconEye,
	"encoders.base64":		IconEncode,
	"encoders.gzip":		IconZip,
	"encoders.htmlenc":		IconClear,
	"encoders.jwt":			IconEncode,
	"encoders.qrcode":		IconTable,
	"encoders.urlenc":		IconGlobe,
	"formatters.jsonfmt":		IconTransform,
	"formatters.sqlfmt":		IconDatabase,
	"formatters.xmlfmt":		IconClear,
	"graphic.colorblind":		IconPaint,
	"graphic.imageconvert":		IconEye,
	"generators.hash":		IconEncode,
	"generators.lorem":		IconFile,
	"generators.password":		IconEncode,
	"generators.uuid":		IconKey,
	"testers.xmlvalidator":		IconClear,
	"testers.jsonpath":		IconSearch,
	"testers.regex":		IconSearch,
	"testers.xpath":		IconSearch,
	"text.listcompare":		IconCompare,
	"text.escape":			IconTransform,
	"text.analyzer":		IconWrench,
	"text.comparer":		IconCompare,
	"text.markdown":		IconBook,
}

// groupIconNames 把类别名映射到语义图标。
var groupIconNames = map[string]IconName{
	"converters":	IconTransform,
	"encoders":	IconCode,
	"formatters":	IconWrench,
	"generators":	IconWrench,
	"graphic":	IconEye,
	"testers":	IconBug,
	"text":		IconFile,
}

var asciiMode = false

func init() {
	if v, err := strconv.ParseBool(os.Getenv("DEVTOYS_ASCII")); err == nil {
		asciiMode = v
	} else if strings.EqualFold(os.Getenv("DEVTOYS_ASCII"), "on") {
		asciiMode = true
	}
}

// SetASCII 强制使用 ASCII 回退图标。
func SetASCII(enabled bool)	{ asciiMode = enabled }

// ASCII 报告当前是否使用 ASCII 回退图标。
func ASCII() bool	{ return asciiMode }

// Icon 按语义名称返回当前模式下的图标字符。
func Icon(name IconName) string {
	if !asciiMode {
		if icon := nerdIcons[name]; icon != "" {
			return icon
		}
	}
	if icon := asciiIcons[name]; icon != "" {
		return icon
	}
	return string(asciiIcons[IconLogo])
}

// IconWidth 用 go-runewidth 返回图标的显示宽度。
func IconWidth(name IconName) int	{ return runewidth.StringWidth(Icon(name)) }

// ToolIcon 按工具注册表 ID 返回图标；未知 ID 回退到应用标识。
func ToolIcon(id string) string {
	if name, ok := toolIconNames[id]; ok {
		return Icon(name)
	}
	return Icon(IconLogo)
}

// GroupIcon 按类别名返回图标；未知类别回退到应用标识。
func GroupIcon(category string) string {
	if name, ok := groupIconNames[category]; ok {
		return Icon(name)
	}
	return Icon(IconLogo)
}

// StatusIcon 按状态严重度（0=info 1=success 2=warning 3=error）返回图标，
// 与 toolkit.StatusSeverity 的取值顺序保持一致。
func StatusIcon(severity uint8) string {
	switch severity {
	case 1:
		return Icon(IconSuccess)
	case 2:
		return Icon(IconWarning)
	case 3:
		return Icon(IconError)
	default:
		return Icon(IconInfo)
	}
}
