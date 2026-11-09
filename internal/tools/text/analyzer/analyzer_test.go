package analyzer

import "testing"

// TestAnalyzeMixed 验证中英混排文本的各项统计。
func TestAnalyzeMixed(t *testing.T) {
	s := analyze("Hello world. 你好！\n\nSecond paragraph.")
	if s.empty {
		t.Fatal("不应判为空")
	}
	// "Hello world. 你好！\n\nSecond paragraph." 共 35 个 rune。
	if s.chars != 35 {
		t.Fatalf("chars=%d, want 35", s.chars)
	}
	// 不含空白字符：去掉 5 个（2 个空格、2 个换行、Second 后的 1 个空格）。
	if s.charsNoSpace != 30 {
		t.Fatalf("charsNoSpace=%d, want 30", s.charsNoSpace)
	}
	// 词：hello, world, 你, 好, second, paragraph = 6。
	if s.words != 6 {
		t.Fatalf("words=%d, want 6", s.words)
	}
	if s.lines != 3 {
		t.Fatalf("lines=%d, want 3", s.lines)
	}
	if s.paragraphs != 2 {
		t.Fatalf("paragraphs=%d, want 2", s.paragraphs)
	}
	// 句：Hello world. / 你好！ / Second paragraph. = 3。
	if s.sentences != 3 {
		t.Fatalf("sentences=%d, want 3", s.sentences)
	}
	// hello 与 world 各出现一次且并列榜首（字母序 hello 在前）。
	if len(s.top) == 0 || s.top[0].word != "hello" {
		t.Fatalf("top[0]=%+v", s.top)
	}
}

// TestAnalyzeEmpty 验证空文本走空态分支。
func TestAnalyzeEmpty(t *testing.T) {
	s := analyze("")
	if !s.empty {
		t.Fatal("空文本应标记 empty")
	}
}

// TestCountParagraphs 验证含空白字符的空行同样分隔段落。
func TestCountParagraphs(t *testing.T) {
	if got := countParagraphs("a\n  \nb\n\nc"); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}
