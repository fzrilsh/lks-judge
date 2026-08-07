package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// StaticHandler returns an http.Handler that serves embedded static files.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// PDFLogos returns the two header logos as bytes for the scoring PDF. Missing
// files return nil, so the PDF just omits that logo.
func PDFLogos() (left, right []byte) {
	left, _ = staticFS.ReadFile("static/imgs/logo.png")
	right, _ = staticFS.ReadFile("static/imgs/logo-worldskills.jpg")
	return left, right
}
