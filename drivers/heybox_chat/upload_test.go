package heybox_chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildInfoFormMatchesUploadImage(t *testing.T) {
	form, err := buildInfoForm(fileInfo{
		MimeType: "image/png",
		FSize:    123456,
		Width:    1920,
		Height:   1080,
		Ext:      "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if form["scope"] != "chat" || form["type"] != "" || form["source"] != "" || form["ext"] != "" || form["need_cache"] != "0" {
		t.Fatalf("info form = %#v", form)
	}
	var infos []fileInfo
	if err := json.Unmarshal([]byte(form["file_infos"]), &infos); err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Ext != "png" || infos[0].MimeType != "image/png" {
		t.Fatalf("file_infos = %s", form["file_infos"])
	}
}

func TestRequestUploadInfoUsesFormURLEncoded(t *testing.T) {
	var (
		gotCT     string
		gotCookie string
		form      map[string][]string
		query     map[string][]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotCookie = r.Header.Get("Cookie")
		body, _ := io.ReadAll(r.Body)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		form = r.PostForm
		query = r.URL.Query()
		if len(form) == 0 && strings.Contains(gotCT, "json") {
			form = map[string][]string{"raw": {string(body)}}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"bucket":"chat-1251007209","keys":["web/chat/2026/08/30/abc.png"],"host":"chat.max-c.com/"}}`))
	}))
	defer srv.Close()

	d := &HeyboxChat{
		Addition: Addition{HeyboxID: "1", Pkey: "secret-pkey"},
		apiHost:  srv.URL,
	}
	out, err := d.requestUploadInfo(context.Background(), fileInfo{
		MimeType: "image/png",
		FSize:    8,
		Width:    8,
		Height:   8,
		Ext:      "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Bucket != "chat-1251007209" || out.Keys[0] != "web/chat/2026/08/30/abc.png" {
		t.Fatalf("result = %+v", out)
	}
	if !strings.Contains(gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q, want urlencoded", gotCT)
	}
	if !strings.Contains(gotCookie, "user_pkey=secret-pkey") {
		t.Fatalf("Cookie = %q, want user_pkey", gotCookie)
	}
	if got := firstForm(form, "scope"); got != "chat" {
		t.Fatalf("scope = %q", got)
	}
	if got := firstForm(form, "type"); got != "" {
		t.Fatalf("type = %q, want empty string", got)
	}
	if got := firstForm(form, "source"); got != "" {
		t.Fatalf("source = %q, want empty string", got)
	}
	if got := firstForm(form, "ext"); got != "" {
		t.Fatalf("top-level ext = %q, want empty string", got)
	}
	if got := firstForm(form, "need_cache"); got != "0" {
		t.Fatalf("need_cache = %q", got)
	}
	var infos []fileInfo
	if err := json.Unmarshal([]byte(firstForm(form, "file_infos")), &infos); err != nil {
		t.Fatalf("file_infos not JSON string: %q err=%v", firstForm(form, "file_infos"), err)
	}
	if len(infos) != 1 || infos[0].Ext != "png" {
		t.Fatalf("file_infos = %#v", infos)
	}
	if firstForm(query, "hkey") == "" || firstForm(query, "device_id") == "" {
		t.Fatalf("query missing sign fields: %v", query)
	}
}

func firstForm(v map[string][]string, key string) string {
	if v == nil || len(v[key]) == 0 {
		return ""
	}
	return v[key][0]
}
