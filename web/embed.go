package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

type fallbackFS struct {
	primary   http.FileSystem
	secondary http.FileSystem
}

func (f *fallbackFS) Open(name string) (http.File, error) {
	file, err := f.primary.Open(name)
	if err == nil {
		return file, nil
	}
	if f.secondary != nil {
		return f.secondary.Open(name)
	}
	return nil, err
}

// DistFS returns an http.FileSystem representing the built frontend
func DistFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// AstFS returns an http.FileSystem representing web/dist/ast with fallback to dist/assets
func AstFS() http.FileSystem {
	subAst, _ := fs.Sub(distFS, "dist/ast")
	subAssets, _ := fs.Sub(distFS, "dist/assets")
	return &fallbackFS{
		primary:   http.FS(subAst),
		secondary: http.FS(subAssets),
	}
}

// AssetsFS returns an http.FileSystem representing web/dist/assets with fallback to dist/ast
func AssetsFS() http.FileSystem {
	subAssets, _ := fs.Sub(distFS, "dist/assets")
	subAst, _ := fs.Sub(distFS, "dist/ast")
	return &fallbackFS{
		primary:   http.FS(subAssets),
		secondary: http.FS(subAst),
	}
}

// ReadIndexHTML returns the index.html content
func ReadIndexHTML() ([]byte, error) {
	return distFS.ReadFile("dist/index.html")
}
