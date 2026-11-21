package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListItems(t *testing.T) {
	run := func(_ context.Context, args ...string) ([]byte, error) {
		switch args[3] {
		case "sinks":
			return []byte(`[{"index":4,"description":"Speakers","mute":false}]`), nil
		case "sink-inputs":
			return []byte(`[{"index":7,"mute":true,"properties":{"application.name":"WEBRTC VoiceEngine","media.name":"playStream"}}]`), nil
		default:
			return []byte(`[]`), nil
		}
	}
	items, err := listItems(run)
	if err != nil || len(items) != 2 {
		t.Fatalf("%+v %v", items, err)
	}
	if items[0].Name != "Speakers" || items[1].Name != "WEBRTC VoiceEngine" || !items[1].Muted || items[1].Detail != "playStream" {
		t.Fatalf("unexpected items: %+v", items)
	}
}
func TestListError(t *testing.T) {
	_, err := listItems(func(context.Context, ...string) ([]byte, error) { return nil, errors.New("failed") })
	if err == nil {
		t.Fatal("expected error")
	}
}
func TestMuteEndpoints(t *testing.T) {
	var calls [][]string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(`[]`), nil
	}
	h := handler("secret", run)
	cases := []struct {
		body   string
		status int
		want   string
	}{{`{"kind":"playback","index":12,"muted":true}`, 204, "set-sink-input-mute 12 1"}, {`{"kind":"recording","index":9,"muted":false}`, 204, "set-source-output-mute 9 0"}, {`{"kind":"outputs","index":0,"muted":false}`, 204, "set-sink-mute 0 0"}, {`{"kind":"inputs","index":1,"muted":true}`, 204, "set-source-mute 1 1"}, {`{"kind":"bad","index":1,"muted":true}`, 400, ""}, {`{"kind":"playback","muted":true}`, 400, ""}, {`{"kind":"playback","index":1}`, 400, ""}, {`{"kind":"playback","index":1,"muted":false,"x":1}`, 400, ""}}
	for _, tc := range cases {
		calls = nil
		r := httptest.NewRequest("POST", "/secret/api/mute", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s status %d", tc.body, w.Code)
		}
		if tc.want != "" && (len(calls) != 1 || strings.Join(calls[0], " ") != tc.want) {
			t.Fatalf("calls: %v", calls)
		}
		if tc.want == "" && len(calls) != 0 {
			t.Fatalf("unexpected calls %v", calls)
		}
	}
}
func TestAPIAndToken(t *testing.T) {
	h := handler("secret", func(context.Context, ...string) ([]byte, error) { return []byte(`[]`), nil })
	for _, tc := range []struct {
		path   string
		status int
	}{{"/secret/", 200}, {"/secret/api/items", 200}, {"/wrong/api/items", 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if strings.HasSuffix(tc.path, "items") && w.Code == 200 {
			var data []Item
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
		}
	}
}
