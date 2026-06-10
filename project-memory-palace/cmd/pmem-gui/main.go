package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atop/project-memory-palace/internal/i18n"
	"github.com/atop/project-memory-palace/internal/service"
)

//go:embed templates/index.html
var indexHTML string

func main() {
	projectRoot := "."
	if len(os.Args) > 1 { projectRoot = os.Args[1] }

	svc := service.New(projectRoot)
	svc.InitProject()

	http.HandleFunc("/api/recent", func(w http.ResponseWriter, r *http.Request) {
		results, err := svc.ListRecent(50)
		writeJSON(w, results, err)
	})
	http.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" { writeJSON(w, nil, nil); return }
		results, err := svc.Recall(q, nil, 30)
		writeJSON(w, results, err)
	})
	http.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		result, err := svc.OpenMemory(id)
		writeJSONRaw(w, result, err)
	})
	http.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { http.Error(w, "POST required", 405); return }
		id := r.URL.Query().Get("id")
		status := r.URL.Query().Get("status")
		if status == "" { http.Error(w, "status required", 400); return }
		result, err := svc.UpdateMemory(id, map[string]any{"status": status})
		writeJSONRaw(w, result, err)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := strings.ReplaceAll(indexHTML, "{{.ProjectRoot}}", projectRoot)
		html = strings.ReplaceAll(html, "{{.Lang}}", i18n.GetLanguage())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})
	http.HandleFunc("/lang/", func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Path[6:]
		i18n.SetLanguage(lang)
		http.Redirect(w, r, "/", 302)
	})

	port := "8147"
	url := "http://localhost:" + port
	fmt.Printf("Project Memory Palace GUI\nOpen %s\n", url)
	openBrowser(url)
	http.ListenAndServe(":"+port, nil)
}

func writeJSON(w http.ResponseWriter, results []map[string]any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil { json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
	if results == nil { results = []map[string]any{} }
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func writeJSONRaw(w http.ResponseWriter, data map[string]any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil { json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
	json.NewEncoder(w).Encode(data)
}

func openBrowser(url string) {
	var cmd string; var args []string
	switch runtime.GOOS {
	case "windows": cmd = "cmd"; args = []string{"/c","start",url}
	case "darwin": cmd = "open"; args = []string{url}
	default: cmd = "xdg-open"; args = []string{url}
	}
	exec.Command(cmd, args...).Start()
}
