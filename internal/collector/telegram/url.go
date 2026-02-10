package telegram

import "regexp"

// urlRe matches HTTP(S) URLs in message text.
var urlRe = regexp.MustCompile(`https?://[^\s<>"'\)\]]+`)

// extractFirstURL returns the first HTTP(S) URL found in the text, or "".
func extractFirstURL(text string) string {
	return urlRe.FindString(text)
}
