package toolkit

// categoryTitles 是类别标识到中文显示名的映射。
var categoryTitles = map[string]string{
	"converters":	"Converters",
	"encoders":	"Encoders",
	"formatters":	"Formatters",
	"generators":	"Generators",
	"graphic":	"Graphic",
	"testers":	"Testers",
	"text":		"Text",
}

// categoryOrder 固定类别在侧边栏中的出现顺序。
var categoryOrder = []string{
	"converters",
	"encoders",
	"formatters",
	"generators",
	"graphic",
	"testers",
	"text",
}

// CategoryTitle 返回类别的显示名；未知类别原样返回。
func CategoryTitle(c string) string {
	if t, ok := categoryTitles[c]; ok {
		return t
	}
	return c
}

// CategoryOrder 返回类别的排序序号；未知类别排在最后。
func CategoryOrder(c string) int {
	for i, name := range categoryOrder {
		if name == c {
			return i
		}
	}
	return len(categoryOrder)
}

// Categories 返回全部已知类别标识（按显示顺序）。
func Categories() []string {
	return append([]string(nil), categoryOrder...)
}
