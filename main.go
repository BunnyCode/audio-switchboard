package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
static void open_window(const char *url) {
    gtk_init(NULL, NULL);
    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "Audio Switchboard");
    gtk_window_set_default_size(GTK_WINDOW(window), 700, 760);
    gtk_window_set_icon_name(GTK_WINDOW(window), "audio-card");
    GtkWidget *view = webkit_web_view_new();
    gtk_container_add(GTK_CONTAINER(window), view);
    g_signal_connect(window, "destroy", G_CALLBACK(gtk_main_quit), NULL);
    webkit_web_view_load_uri(WEBKIT_WEB_VIEW(view), url);
    gtk_widget_show_all(window);
    gtk_main();
}
*/
import "C"

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

//go:embed ui.html
var assets embed.FS

type Item struct {
	Index  uint32 `json:"index"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Muted  bool   `json:"muted"`
	Kind   string `json:"kind"`
}
type rawItem struct {
	Index       uint32         `json:"index"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Mute        bool           `json:"mute"`
	Properties  map[string]any `json:"properties"`
}
type runner func(context.Context, ...string) ([]byte, error)

var categories = []struct{ Kind, List, Set string }{{"outputs", "sinks", "set-sink-mute"}, {"inputs", "sources", "set-source-mute"}, {"playback", "sink-inputs", "set-sink-input-mute"}, {"recording", "source-outputs", "set-source-output-mute"}}

func command(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "pactl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pactl: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return out, nil
}
func prop(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func listItems(run runner) ([]Item, error) {
	items := make([]Item, 0)
	for _, cat := range categories {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		out, err := run(ctx, "-f", "json", "list", cat.List)
		cancel()
		if err != nil {
			return nil, err
		}
		var raw []rawItem
		if err = json.Unmarshal(out, &raw); err != nil {
			return nil, fmt.Errorf("reading %s: %w", cat.List, err)
		}
		for _, r := range raw {
			name := r.Description
			if cat.Kind == "playback" || cat.Kind == "recording" {
				name = prop(r.Properties, "application.name", "media.name", "node.name")
			}
			if name == "" {
				name = prop(r.Properties, "device.description", "node.description", "node.nick")
			}
			if name == "" {
				name = r.Name
			}
			if name == "" {
				name = fmt.Sprintf("Stream %d", r.Index)
			}
			detail := prop(r.Properties, "media.name")
			if detail == name {
				detail = ""
			}
			items = append(items, Item{Index: r.Index, Name: name, Detail: detail, Muted: r.Mute, Kind: cat.Kind})
		}
	}
	return items, nil
}
func handler(token string, run runner) http.Handler {
	mux := http.NewServeMux()
	prefix := "/" + token
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix+"/" || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		b, _ := assets.ReadFile("ui.html")
		_, _ = w.Write(b)
	})
	mux.HandleFunc(prefix+"/api/items", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		items, err := listItems(run)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(items)
	})
	mux.HandleFunc(prefix+"/api/mute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON required", 415)
			return
		}
		var req struct {
			Kind  string  `json:"kind"`
			Index *uint32 `json:"index"`
			Muted *bool   `json:"muted"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 2048))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil || req.Index == nil || req.Muted == nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		op := ""
		for _, c := range categories {
			if c.Kind == req.Kind {
				op = c.Set
				break
			}
		}
		if op == "" {
			http.Error(w, "Invalid kind", 400)
			return
		}
		value := "0"
		if *req.Muted {
			value = "1"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		_, err := run(ctx, op, strconv.FormatUint(uint64(*req.Index), 10), value)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(204)
	})
	return mux
}
func main() {
	runtime.LockOSThread()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	token := hex.EncodeToString(b)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: handler(token, command), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	url := "http://" + listener.Addr().String() + "/" + token + "/"
	if len(os.Args) > 1 && os.Args[1] == "--headless" {
		fmt.Println(url)
		select {}
	}
	c := C.CString(url)
	C.open_window(c)
	C.free(unsafe.Pointer(c))
	_ = server.Close()
}
