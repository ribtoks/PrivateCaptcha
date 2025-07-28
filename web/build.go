package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

//go:embed static
var staticFiles embed.FS

func StaticFiles() *embed.FS {
	return &staticFiles
}

func Static() http.HandlerFunc {
	sub, _ := fs.Sub(staticFiles, "static")
	srv := http.FileServer(http.FS(sub))

	return func(w http.ResponseWriter, r *http.Request) {
		slog.DebugContext(r.Context(), "Static request", "path", r.URL.Path)
		common.WriteHeaders(w, common.CachedHeaders)
		common.WriteHeaders(w, common.SecurityHeaders)
		srv.ServeHTTP(w, r)
	}
}

//go:embed layouts/*/*.html
var templateFiles embed.FS

func Templates() *embed.FS {
	return &templateFiles
}

//go:embed data/*.json
var dataFiles embed.FS

type DataContext map[string]interface{}

func LoadData() (DataContext, error) {
	data := make(DataContext)

	entries, err := dataFiles.ReadDir("data")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		content, err := dataFiles.ReadFile("data/" + entry.Name())
		if err != nil {
			return nil, err
		}

		var parsed interface{}
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, err
		}

		key := strings.TrimSuffix(entry.Name(), ".json")
		data[key] = parsed
	}

	return data, nil
}
