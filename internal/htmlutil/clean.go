package htmlutil

import (
	"github.com/k3a/html2text"
)

// ToText converts HTML to plain text using a proper HTML parser.
// Handles entities, strips tags, and preserves readable text.
func ToText(s string) string {
	return html2text.HTML2Text(s)
}
