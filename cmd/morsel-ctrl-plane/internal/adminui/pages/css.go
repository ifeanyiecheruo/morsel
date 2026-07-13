package pages

import (
	_ "embed"

	"github.com/a-h/templ"
)

//go:embed admin.css
var adminCSS string

//go:embed operation-poll.js
var operationPollJS string

// inlineCSS returns a component that injects the admin stylesheet as an inline
// <style> block. Used by Layout and LoginPage since templ does not interpolate
// Go expressions inside <style> element content.
func inlineCSS() templ.Component {
	return templ.Raw("<style>" + adminCSS + "</style>")
}

// inlineOperationPollJS injects the operation-status polling script. Used only
// by OperationStatusPage — the rest of the admin UI ships no JavaScript.
func inlineOperationPollJS() templ.Component {
	return templ.Raw("<script>" + operationPollJS + "</script>")
}
