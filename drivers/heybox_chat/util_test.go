package heybox_chat

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
)

func TestJoinPublicURL(t *testing.T) {
	tests := []struct {
		host, key, want string
	}{
		{"chat.max-c.com/", "oa/2026/08/30/abc.png", "https://chat.max-c.com/oa/2026/08/30/abc.png"},
		{"chat.max-c.com", "oa/abc.png", "https://chat.max-c.com/oa/abc.png"},
		{"https://imgheybox.max-c.com/", "oa/abc.png", "https://imgheybox.max-c.com/oa/abc.png"},
		{"https://chat.max-c.com", "oa/abc.png", "https://chat.max-c.com/oa/abc.png"},
	}
	for _, tt := range tests {
		got := joinPublicURL(tt.host, tt.key)
		if got != tt.want {
			t.Fatalf("joinPublicURL(%q, %q) = %q, want %q", tt.host, tt.key, got, tt.want)
		}
	}
}

func TestNormalizeExt(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"photo.JPEG", "jpg"},
		{"photo.jpeg", "jpg"},
		{"a.PNG", "png"},
		{"b.gif", "gif"},
		{"noext", ""},
		{"archive.tar.gz", "gz"},
	}
	for _, tt := range tests {
		got := normalizeExt(tt.name)
		if got != tt.want {
			t.Fatalf("normalizeExt(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMarshalJSONString(t *testing.T) {
	raw := []fileInfo{{
		MimeType: "image/png",
		FSize:    123456,
		Width:    1920,
		Height:   1080,
	}}
	got, err := marshalJSONString(raw)
	if err != nil {
		t.Fatal(err)
	}
	var parsed []fileInfo
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result is not a JSON string of an array: %q err=%v", got, err)
	}
	if len(parsed) != 1 || parsed[0].Width != 1920 || parsed[0].FSize != 123456 {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestIndexRoundTrip(t *testing.T) {
	root := &Node{
		Name: "root",
		Children: []*Node{
			{Name: "pics", Children: []*Node{
				{Name: "a.png", URL: "https://chat.max-c.com/oa/a.png", Key: "oa/a.png", Host: "chat.max-c.com/", Size: 10, Mime: "image/png"},
			}},
		},
	}
	text, err := stringifyIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseIndex(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "root" || len(got.Children) != 1 || got.Children[0].Name != "pics" {
		t.Fatalf("round trip root mismatch: %+v", got)
	}
	file := got.Children[0].Children[0]
	if file.URL == "" || file.Key != "oa/a.png" || file.Size != 10 {
		t.Fatalf("round trip file mismatch: %+v", file)
	}
}

func TestParseIndexEmpty(t *testing.T) {
	root, err := parseIndex("")
	if err != nil {
		t.Fatal(err)
	}
	if root == nil || root.Name != "root" || root.URL != "" || len(root.Children) != 0 {
		t.Fatalf("empty index should be an empty root, got %+v", root)
	}
}

func TestFindChildAndCollision(t *testing.T) {
	root, err := parseIndex("")
	if err != nil {
		t.Fatal(err)
	}
	if err := addChild(root, &Node{Name: "a.png", URL: "https://x/a.png"}); err != nil {
		t.Fatal(err)
	}
	if err := addChild(root, &Node{Name: "a.png", URL: "https://x/b.png"}); !errors.Is(err, errs.ObjectAlreadyExists) {
		t.Fatalf("want ObjectAlreadyExists, got %v", err)
	}
	got := childByName(root, "a.png")
	if got == nil || got.URL != "https://x/a.png" {
		t.Fatalf("childByName = %+v", got)
	}
}

func TestRemoveChild(t *testing.T) {
	root, err := parseIndex("")
	if err != nil {
		t.Fatal(err)
	}
	_ = addChild(root, &Node{Name: "a.png", URL: "https://x/a.png"})
	if err := removeChild(root, "a.png"); err != nil {
		t.Fatal(err)
	}
	if childByName(root, "a.png") != nil {
		t.Fatal("child still present")
	}
	if err := removeChild(root, "a.png"); !errors.Is(err, errs.ObjectNotFound) {
		t.Fatalf("want ObjectNotFound, got %v", err)
	}
}
