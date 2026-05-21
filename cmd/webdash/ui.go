// ui.go — Serve os arquivos estáticos via go:embed (diretório static/).
package main

import "embed"

//go:embed static
var staticFS embed.FS
