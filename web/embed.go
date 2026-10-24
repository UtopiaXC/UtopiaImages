package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var distFS embed.FS

// DistFS returns an http.FileSystem representing the built frontend
func DistFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// ReadIndexHTML returns the index.html content
func ReadIndexHTML() ([]byte, error) {
	return distFS.ReadFile("dist/index.html")
}
