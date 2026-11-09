package markdown

import (
	"bytes"
	"html"
	"strings"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	emojiast "github.com/yuin/goldmark-emoji/ast"
	"github.com/yuin/goldmark-emoji/definition"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// termRenderer combines Glamour's terminal renderer with the extensions used
// by the desktop Markdown preview. Each background job owns its renderer.
type termRenderer struct{ md goldmark.Markdown }

func newRenderer(width int, light bool) (*termRenderer, error) {
	style := markdownStyle()
	if light {
		style = lightMarkdownStyle()
	}
	terminal := ansi.NewRenderer(ansi.Options{WordWrap: width, PreserveNewLines: true, ColorProfile: termenv.TrueColor, Styles: style})
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList, extension.Footnote, extension.Typographer, emoji.Emoji),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithASTTransformers(util.Prioritized(terminalStrings{}, 999))),
	)
	// Extensions install HTML node renderers as a side effect; replace them,
	// as Glamour does, so only the terminal renderer writes to the output.
	md.SetRenderer(renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(terminal, 1000))))
	return &termRenderer{md: md}, nil
}

// Glamour renders emoji inline nodes but does not implement Goldmark's String
// nodes. Typographer emits String nodes for punctuation, so adapt those nodes
// to Glamour's existing inline-unicode representation after parsing.
type terminalStrings struct{}

func (terminalStrings) Transform(root *ast.Document, _ text.Reader, _ parser.Context) {
	var stringsToReplace []*ast.String
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == ast.KindString {
			stringsToReplace = append(stringsToReplace, node.(*ast.String))
		}
		return ast.WalkContinue, nil
	})
	for _, node := range stringsToReplace {
		value := html.UnescapeString(string(node.Value))
		parent := node.Parent()
		parent.ReplaceChild(parent, node, emojiast.NewEmoji(nil, &definition.Emoji{Unicode: []rune(value)}))
	}
}

func (r *termRenderer) Render(source string) (string, error) {
	var output bytes.Buffer
	err := r.md.Convert([]byte(withoutFrontMatter(source)), &output)
	return output.String(), err
}

// YAML front matter is metadata rather than visible document content. Leave an
// unclosed block untouched so an ordinary horizontal rule is never discarded.
func withoutFrontMatter(source string) string {
	first, rest, ok := nextMarkdownLine(source)
	if !ok || strings.TrimPrefix(first, "\ufeff") != "---" {
		return source
	}
	remaining := rest
	for len(remaining) > 0 {
		line, next, hasNewline := nextMarkdownLine(remaining)
		if line == "---" || line == "..." {
			return next
		}
		if !hasNewline {
			break
		}
		remaining = next
	}
	return source
}

func nextMarkdownLine(source string) (line, rest string, hasNewline bool) {
	i := strings.IndexAny(source, "\r\n")
	if i < 0 {
		return source, "", false
	}
	end := i + 1
	if source[i] == '\r' && end < len(source) && source[end] == '\n' {
		end++
	}
	return source[:i], source[end:], true
}
