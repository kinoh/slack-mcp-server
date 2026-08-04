package text

import (
	"crypto/x509"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"go.uber.org/zap"
	"golang.org/x/net/publicsuffix"
)

func AttachmentToText(att slack.Attachment) string {
	var parts []string

	if att.Title != "" {
		parts = append(parts, fmt.Sprintf("Title: %s", att.Title))
	}

	if att.AuthorName != "" {
		parts = append(parts, fmt.Sprintf("Author: %s", att.AuthorName))
	}

	if att.Pretext != "" {
		parts = append(parts, fmt.Sprintf("Pretext: %s", att.Pretext))
	}

	if att.Text != "" {
		parts = append(parts, fmt.Sprintf("Text: %s", att.Text))
	}

	if att.Footer != "" {
		ts, _ := TimestampToIsoRFC3339(string(att.Ts) + ".000000")

		parts = append(parts, fmt.Sprintf("Footer: %s @ %s", att.Footer, ts))
	}

	result := strings.Join(parts, "; ")

	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.ReplaceAll(result, "\r", " ")
	result = strings.ReplaceAll(result, "\t", " ")
	result = strings.ReplaceAll(result, "(", "[")
	result = strings.ReplaceAll(result, ")", "]")
	result = strings.TrimSpace(result)

	return result
}

func AttachmentsTo2CSV(msgText string, attachments []slack.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var descriptions []string
	for _, att := range attachments {
		plainText := AttachmentToText(att)
		if plainText != "" {
			descriptions = append(descriptions, fmt.Sprintf("%s", plainText))
		}
	}

	prefix := ""
	if msgText != "" {
		prefix = ". "
	}

	return prefix + strings.Join(descriptions, ", ")
}

// MessageToMarkdown returns a display-oriented representation without reusing
// the flattened search text. Slack mrkdwn is close to Markdown, so the message
// body is retained and Slack-specific links are converted explicitly.
func MessageToMarkdown(msgText string, attachments []slack.Attachment) string {
	var sections []string
	if body := strings.TrimSpace(SlackMrkdwnToMarkdown(msgText)); body != "" {
		sections = append(sections, body)
	}

	var renderedAttachments []string
	for _, attachment := range attachments {
		if rendered := AttachmentToMarkdown(attachment); rendered != "" {
			renderedAttachments = append(renderedAttachments, rendered)
		}
	}
	if len(renderedAttachments) > 0 {
		sections = append(sections, strings.Join(renderedAttachments, "\n\n---\n\n"))
	}

	return strings.Join(sections, "\n\n")
}

// AttachmentToMarkdown preserves the semantic boundaries Slack exposes for an
// attachment instead of collapsing every field into a single CSV-oriented line.
func AttachmentToMarkdown(att slack.Attachment) string {
	var sections []string

	if att.Title != "" {
		title := SlackMrkdwnToMarkdown(att.Title)
		if link := markdownLink(title, att.TitleLink); link != "" {
			title = link
		}
		sections = append(sections, "## "+title)
	}
	if att.AuthorName != "" {
		author := SlackMrkdwnToMarkdown(att.AuthorName)
		if link := markdownLink(author, att.AuthorLink); link != "" {
			author = link
		}
		sections = append(sections, "**Author:** "+author)
	}
	if att.Pretext != "" {
		sections = append(sections, SlackMrkdwnToMarkdown(att.Pretext))
	}
	if att.Text != "" {
		sections = append(sections, SlackMrkdwnToMarkdown(att.Text))
	}
	for _, field := range att.Fields {
		var fieldParts []string
		if field.Title != "" {
			fieldParts = append(fieldParts, "**"+SlackMrkdwnToMarkdown(field.Title)+"**")
		}
		if field.Value != "" {
			fieldParts = append(fieldParts, SlackMrkdwnToMarkdown(field.Value))
		}
		if len(fieldParts) > 0 {
			sections = append(sections, strings.Join(fieldParts, "\n\n"))
		}
	}
	if att.Footer != "" {
		footer := SlackMrkdwnToMarkdown(att.Footer)
		if att.Ts != "" {
			rawTimestamp := string(att.Ts)
			if timestamp, err := TimestampToIsoRFC3339(rawTimestamp + ".000000"); err == nil {
				footer += " @ " + timestamp
			} else {
				footer += " @ " + rawTimestamp
			}
		}
		sections = append(sections, footer)
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

var (
	slackURLLinkRegex     = regexp.MustCompile(`<((?:https?://|mailto:)[^>|]+)(?:\|([^>]+))?>`)
	slackChannelLinkRegex = regexp.MustCompile(`<#([^>|]+)(?:\|([^>]+))?>`)
	slackUserLinkRegex    = regexp.MustCompile(`<@([^>|]+)(?:\|([^>]+))?>`)
	slackSpecialLinkRegex = regexp.MustCompile(`<!([^>|]+)(?:\|([^>]+))?>`)
)

// SlackMrkdwnToMarkdown retains newlines, lists, quotes, and code while making
// Slack-specific links and mentions readable by a standard Markdown renderer.
func SlackMrkdwnToMarkdown(value string) string {
	value = slackURLLinkRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := slackURLLinkRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		if len(parts) < 3 || parts[2] == "" {
			return parts[1]
		}
		return markdownLink(parts[2], parts[1])
	})
	value = slackChannelLinkRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := slackChannelLinkRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		name := parts[1]
		if len(parts) == 3 && parts[2] != "" {
			name = parts[2]
		}
		return "#" + strings.TrimPrefix(name, "#")
	})
	value = slackUserLinkRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := slackUserLinkRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		name := parts[1]
		if len(parts) == 3 && parts[2] != "" {
			name = parts[2]
		}
		return "@" + strings.TrimPrefix(name, "@")
	})
	value = slackSpecialLinkRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := slackSpecialLinkRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		name := parts[1]
		if len(parts) == 3 && parts[2] != "" {
			name = parts[2]
		}
		return "@" + strings.TrimPrefix(name, "@")
	})
	return value
}

func markdownLink(label, rawURL string) string {
	if label == "" || rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "mailto") {
		return ""
	}
	escapedLabel := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(label)
	return fmt.Sprintf("[%s](%s)", escapedLabel, rawURL)
}

func IsUnfurlingEnabled(text string, opt string, logger *zap.Logger) bool {
	if opt == "" || opt == "no" || opt == "false" || opt == "0" {
		return false
	}

	if opt == "yes" || opt == "true" || opt == "1" {
		return true
	}

	allowed := make(map[string]struct{}, 0)
	for _, d := range strings.Split(opt, ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		allowed[d] = struct{}{}
	}

	urlRe := regexp.MustCompile(`https?://[^\s]+`)
	urls := urlRe.FindAllString(text, -1)
	for _, rawURL := range urls {
		u, err := url.Parse(rawURL)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		host = strings.TrimPrefix(host, "www.")
		if _, ok := allowed[host]; !ok {
			if logger != nil {
				logger.Warn("Security: attempt to unfurl non-whitelisted host",
					zap.String("host", host),
					zap.String("allowed", opt),
				)
			}
			return false
		}
	}

	txtNoURLs := urlRe.ReplaceAllString(text, " ")

	domRe := regexp.MustCompile(`\b(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+[A-Za-z]{2,}\b`)
	doms := domRe.FindAllString(txtNoURLs, -1)

	for _, d := range doms {
		d = strings.ToLower(d)

		if _, icann := publicsuffix.PublicSuffix(d); !icann {
			continue
		}

		if _, ok := allowed[d]; !ok {
			if logger != nil {
				logger.Warn("Security: attempt to unfurl non-whitelisted host",
					zap.String("host", d),
					zap.String("allowed", opt),
				)
			}
			return false
		}
	}

	return true
}

func Workspace(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid Slack URL: %q", rawURL)
	}
	return parts[0], nil
}

func TimestampToIsoRFC3339(slackTS string) (string, error) {
	parts := strings.Split(slackTS, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid slack timestamp format: %s", slackTS)
	}

	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse seconds: %v", err)
	}

	microseconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse microseconds: %v", err)
	}

	t := time.Unix(seconds, microseconds*1000)

	return t.UTC().Format(time.RFC3339), nil
}

func ProcessText(s string) string {
	s = filterSpecialChars(s)

	return s
}

func HumanizeCertificates(certs []*x509.Certificate) string {
	var descriptions []string
	for _, cert := range certs {
		subjectCN := cert.Subject.CommonName
		issuerCN := cert.Issuer.CommonName
		expiry := cert.NotAfter.Format("2006-01-02")

		description := fmt.Sprintf("CN=%s (Issuer CN=%s, expires %s)", subjectCN, issuerCN, expiry)
		descriptions = append(descriptions, description)
	}
	return strings.Join(descriptions, ", ")
}

func filterSpecialChars(text string) string {
	replaceWithCommaCheck := func(match []string, isLast bool) string {
		var url, linkText string

		if len(match) == 3 && strings.Contains(match[0], "|") {
			url = match[1]
			linkText = match[2]
		} else if len(match) == 3 {
			linkText = match[1]
			url = match[2]
		}

		replacement := url + " - " + linkText

		if !isLast {
			replacement += ","
		}

		return replacement
	}

	// Helper function to check if this is the last link/element
	isLastInText := func(original string, currentText string) bool {
		linkPos := strings.LastIndex(currentText, original)
		if linkPos == -1 {
			return false
		}
		afterLink := strings.TrimSpace(currentText[linkPos+len(original):])
		return afterLink == ""
	}

	// Handle Slack-style links: <URL|Description>
	slackLinkRegex := regexp.MustCompile(`<(https?://[^>|]+)\|([^>]+)>`)
	slackMatches := slackLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range slackMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		replacement := replaceWithCommaCheck(match, isLast)
		text = strings.Replace(text, original, replacement, 1)
	}

	// Handle markdown links: [Description](URL)
	markdownLinkRegex := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	markdownMatches := markdownLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range markdownMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		replacement := replaceWithCommaCheck(match, isLast)
		text = strings.Replace(text, original, replacement, 1)
	}

	htmlLinkRegex := regexp.MustCompile(`<a\s+href=["']([^"']+)["'][^>]*>([^<]+)</a>`)
	htmlMatches := htmlLinkRegex.FindAllStringSubmatch(text, -1)
	for _, match := range htmlMatches {
		original := match[0]
		isLast := isLastInText(original, text)
		url := match[1]
		linkText := match[2]
		replacement := url + " - " + linkText
		if !isLast {
			replacement += ","
		}
		text = strings.Replace(text, original, replacement, 1)
	}

	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	urls := urlRegex.FindAllString(text, -1)

	protected := text
	for i, url := range urls {
		placeholder := "___URL_PLACEHOLDER_" + string(rune(48+i)) + "___"
		protected = strings.Replace(protected, url, placeholder, 1)
	}

	cleanRegex := regexp.MustCompile(`[^0-9\p{L}\p{M}\s\.\,\-_:/\?=&%]`)
	cleaned := cleanRegex.ReplaceAllString(protected, "")

	// Restore the URLs
	for i, url := range urls {
		placeholder := "___URL_PLACEHOLDER_" + string(rune(48+i)) + "___"
		cleaned = strings.Replace(cleaned, placeholder, url, 1)
	}

	spaceRegex := regexp.MustCompile(`[ \t]+`)
	cleaned = spaceRegex.ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}
