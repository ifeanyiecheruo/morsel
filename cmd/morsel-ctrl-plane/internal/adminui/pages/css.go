package pages

import (
	_ "embed"

	"github.com/a-h/templ"
)

//go:embed admin.css
var adminCSS string

// inlineCSS returns a component that injects the admin stylesheet as an inline
// <style> block. Used by Layout and LoginPage since templ does not interpolate
// Go expressions inside <style> element content.
func inlineCSS() templ.Component {
	return templ.Raw("<style>" + adminCSS + "</style>")
}
