package tui

import (
	"strings"
	"testing"
)

func TestPreprocessGitHubHTML_BrTags(t *testing.T) {
	input := "line one<br>line two<br/>line three<br />"
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "line one\nline two\nline three") {
		t.Errorf("should convert <br> to newlines, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_Details(t *testing.T) {
	input := "<details><summary>Click to expand</summary>Hidden content here</details>"
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "**Click to expand**") {
		t.Errorf("should convert summary to bold, got: %q", result)
	}
	if !strings.Contains(result, "Hidden content here") {
		t.Errorf("should preserve details body, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_Img(t *testing.T) {
	input := `Check this <img src="https://example.com/img.png" alt="screenshot"> for reference`
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "screenshot") {
		t.Errorf("should extract alt text from img, got: %q", result)
	}
	if strings.Contains(result, "<img") {
		t.Errorf("should strip img tag, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_InlineCode(t *testing.T) {
	input := "use <code>go build</code> to compile"
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "`go build`") {
		t.Errorf("should convert <code> to backticks, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_Bold(t *testing.T) {
	input := "this is <b>bold</b> and <strong>strong</strong>"
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "**bold**") || !strings.Contains(result, "**strong**") {
		t.Errorf("should convert bold/strong tags, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_Link(t *testing.T) {
	input := `click <a href="https://example.com">here</a> for details`
	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "[here](https://example.com)") {
		t.Errorf("should convert links to markdown, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_StripRemaining(t *testing.T) {
	input := "text <div>inside div</div> and <span>span</span>"
	result := preprocessGitHubHTML(input)
	if strings.Contains(result, "<div>") || strings.Contains(result, "<span>") {
		t.Errorf("should strip remaining HTML tags, got: %q", result)
	}
	if !strings.Contains(result, "inside div") || !strings.Contains(result, "span") {
		t.Errorf("should preserve text content, got: %q", result)
	}
}

func TestPreprocessGitHubHTML_MixedContent(t *testing.T) {
	input := `**Bold markdown** with <code>html code</code> and a <br>line break

- list item 1
- list item 2

<details>
<summary>Expand</summary>
Some <b>details</b> here
</details>`

	result := preprocessGitHubHTML(input)
	if !strings.Contains(result, "**Bold markdown**") {
		t.Error("should preserve markdown bold")
	}
	if !strings.Contains(result, "`html code`") {
		t.Error("should convert code tags")
	}
	if !strings.Contains(result, "- list item 1") {
		t.Error("should preserve markdown lists")
	}
	if !strings.Contains(result, "**Expand**") {
		t.Error("should convert details summary")
	}
}

func TestRenderMarkdown_Basic(t *testing.T) {
	input := "**Hello** world\n\nThis is a `test`."
	result := renderMarkdown(input, 60)
	if result == "" {
		t.Error("should produce non-empty output")
	}
	// Glamour should render something (exact ANSI codes vary, but text is present)
	if !strings.Contains(result, "Hello") {
		t.Error("should contain the text 'Hello'")
	}
	if !strings.Contains(result, "test") {
		t.Error("should contain the text 'test'")
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	result := renderMarkdown("", 60)
	if result != "" {
		t.Errorf("empty input should produce empty output, got: %q", result)
	}
}

func TestRenderMarkdown_FallsBackOnNarrowWidth(t *testing.T) {
	// Even with very narrow width, should not panic
	result := renderMarkdown("Some **text** here", 5)
	if result == "" {
		t.Error("should produce output even with narrow width")
	}
}
