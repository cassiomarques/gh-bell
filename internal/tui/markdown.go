package tui

import (
	"regexp"
	"strings"
	"sync"

	glamour "charm.land/glamour/v2"
)

// glamourCache caches glamour renderers by word-wrap width to avoid
// re-creating them on every call. Creating a new TermRenderer involves
// parsing style JSON and building template objects — expensive when
// called multiple times per frame.
var (
	glamourRenderers = make(map[int]*glamour.TermRenderer)
	glamourMu        sync.Mutex
)

func getGlamourRenderer(width int) (*glamour.TermRenderer, error) {
	glamourMu.Lock()
	defer glamourMu.Unlock()
	if r, ok := glamourRenderers[width]; ok {
		return r, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	glamourRenderers[width] = r
	return r, nil
}

// renderMarkdown converts a markdown/HTML string into styled terminal output
// using glamour. Falls back to plain text if rendering fails.
func renderMarkdown(content string, width int) string {
	if content == "" {
		return ""
	}

	// Pre-process GitHub-specific HTML patterns into markdown equivalents
	// before passing to glamour, since glamour strips most raw HTML.
	processed := preprocessGitHubHTML(content)

	renderWidth := width
	if renderWidth < 10 {
		renderWidth = 10
	}

	renderer, err := getGlamourRenderer(renderWidth)
	if err != nil {
		return wordWrap2(processed, width)
	}

	rendered, err := renderer.Render(processed)
	if err != nil {
		return wordWrap2(processed, width)
	}

	return strings.TrimRight(rendered, "\n")
}

// preprocessGitHubHTML converts common GitHub HTML tags into markdown
// equivalents so glamour can render them properly. GitHub comments often
// mix raw HTML with markdown — e.g. <details>, <br>, <img>, <code>.
func preprocessGitHubHTML(content string) string {
	// <br> / <br/> → newline
	content = reBr.ReplaceAllString(content, "\n")

	// <details><summary>Title</summary>Body</details> → bold title + body
	content = reDetails.ReplaceAllStringFunc(content, func(match string) string {
		subs := reDetails.FindStringSubmatch(match)
		if len(subs) == 3 {
			summary := strings.TrimSpace(subs[1])
			body := strings.TrimSpace(subs[2])
			if summary != "" && body != "" {
				return "**" + summary + "**\n\n" + body
			}
			if summary != "" {
				return "**" + summary + "**"
			}
			return body
		}
		return match
	})

	// <img ... alt="description" ...> → [description]
	content = reImg.ReplaceAllString(content, "🖼 $1")

	// <code>text</code> → `text` (inline)
	content = reInlineCode.ReplaceAllString(content, "`$1`")

	// <pre>text</pre> → fenced code block
	content = rePre.ReplaceAllString(content, "\n```\n$1\n```\n")

	// <b>text</b> or <strong>text</strong> → **text**
	content = reBold.ReplaceAllString(content, "**$1**")
	content = reStrong.ReplaceAllString(content, "**$1**")

	// <i>text</i> or <em>text</em> → *text*
	content = reItalic.ReplaceAllString(content, "*$1*")
	content = reEm.ReplaceAllString(content, "*$1*")

	// <a href="url">text</a> → [text](url)
	content = reLink.ReplaceAllString(content, "[$2]($1)")

	// <p>text</p> → text with paragraph breaks
	content = reP.ReplaceAllString(content, "\n\n$1\n\n")

	// <h1>-<h6> → markdown headings
	content = reH1.ReplaceAllString(content, "\n# $1\n")
	content = reH2.ReplaceAllString(content, "\n## $1\n")
	content = reH3.ReplaceAllString(content, "\n### $1\n")

	// <li>text</li> → - text
	content = reLi.ReplaceAllString(content, "- $1")

	// <blockquote>text</blockquote> → > text
	content = reBlockquote.ReplaceAllStringFunc(content, func(match string) string {
		subs := reBlockquote.FindStringSubmatch(match)
		if len(subs) == 2 {
			lines := strings.Split(strings.TrimSpace(subs[1]), "\n")
			for i, l := range lines {
				lines[i] = "> " + strings.TrimSpace(l)
			}
			return strings.Join(lines, "\n")
		}
		return match
	})

	// Strip any remaining HTML tags glamour can't handle
	content = reStripTags.ReplaceAllString(content, "")

	// Collapse excessive blank lines
	content = reExcessiveNewlines.ReplaceAllString(content, "\n\n")

	return strings.TrimSpace(content)
}

// wordWrap2 is a simple plain-text fallback when glamour rendering fails.
func wordWrap2(text string, maxWidth int) string {
	lines := wordWrap(text, maxWidth)
	return strings.Join(lines, "\n")
}

// Compiled regexes for HTML pre-processing (compiled once for performance)
var (
	reBr                = regexp.MustCompile(`<br\s*/?>`)
	reDetails           = regexp.MustCompile(`(?si)<details>\s*<summary>(.*?)</summary>(.*?)</details>`)
	reImg               = regexp.MustCompile(`(?i)<img[^>]*alt="([^"]*)"[^>]*/?>`)
	reInlineCode        = regexp.MustCompile(`(?s)<code>(.*?)</code>`)
	rePre               = regexp.MustCompile(`(?s)<pre>(.*?)</pre>`)
	reBold              = regexp.MustCompile(`(?s)<b>(.*?)</b>`)
	reStrong            = regexp.MustCompile(`(?s)<strong>(.*?)</strong>`)
	reItalic            = regexp.MustCompile(`(?s)<i>(.*?)</i>`)
	reEm                = regexp.MustCompile(`(?s)<em>(.*?)</em>`)
	reLink              = regexp.MustCompile(`(?s)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	reP                 = regexp.MustCompile(`(?s)<p>(.*?)</p>`)
	reH1                = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	reH2                = regexp.MustCompile(`(?s)<h2[^>]*>(.*?)</h2>`)
	reH3                = regexp.MustCompile(`(?s)<h3[^>]*>(.*?)</h3>`)
	reLi                = regexp.MustCompile(`(?s)<li>(.*?)</li>`)
	reBlockquote        = regexp.MustCompile(`(?s)<blockquote>(.*?)</blockquote>`)
	reStripTags         = regexp.MustCompile(`<[^>]*>`)
	reExcessiveNewlines = regexp.MustCompile(`\n{3,}`)
)
