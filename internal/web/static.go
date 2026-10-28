package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets/app.css assets/app.js
var assetsFS embed.FS

// staticHandler serve os assets embutidos em /static/. Eles não são sensíveis
// e podem ficar em cache curto; o CSS/JS ainda não tem hash no nome.
func staticHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic("web: assets embutidos inválidos: " + err.Error())
	}
	arquivos := http.FileServer(http.FS(sub))
	return http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		// Tipo explícito: mime.TypeByExtension varia com o sistema (no Windows
		// .js vira application/javascript). O contrato da UI é text/javascript.
		switch {
		case strings.HasSuffix(r.URL.Path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(r.URL.Path, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		arquivos.ServeHTTP(w, r)
	}))
}
