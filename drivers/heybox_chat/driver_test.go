package heybox_chat

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func newTestDriver(t *testing.T) *HeyboxChat {
	t.Helper()
	d := &HeyboxChat{
		Addition: Addition{
			HeyboxID: "1",
			Pkey:     "x",
		},
	}
	if err := d.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return d
}

func rootDir() model.Obj {
	return &model.Object{Name: "root", Path: "/", IsFolder: true}
}

func TestInitRequiresCredentials(t *testing.T) {
	d := &HeyboxChat{}
	if err := d.Init(context.Background()); err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestLocalTreeOps(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	dir, err := d.MakeDir(ctx, rootDir(), "pics")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.MakeDir(ctx, rootDir(), "pics"); !errors.Is(err, errs.ObjectAlreadyExists) {
		t.Fatalf("want ObjectAlreadyExists, got %v", err)
	}

	d.root.Children[0].Children = append(d.root.Children[0].Children, &Node{
		Name: "a.png",
		URL:  "https://chat.max-c.com/oa/a.png",
		Key:  "oa/a.png",
		Size: 12,
	})

	listed, err := d.List(ctx, dir, model.ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].GetName() != "a.png" || listed[0].IsDir() {
		t.Fatalf("list = %+v", listed)
	}

	link, err := d.Link(ctx, listed[0], model.LinkArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://chat.max-c.com/oa/a.png" {
		t.Fatalf("link = %q", link.URL)
	}

	renamed, err := d.Rename(ctx, listed[0], "b.png")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.GetName() != "b.png" || renamed.GetPath() != "/pics/b.png" {
		t.Fatalf("rename = %+v", renamed)
	}

	moved, err := d.Move(ctx, renamed, rootDir())
	if err != nil {
		t.Fatal(err)
	}
	if moved.GetPath() != "/b.png" {
		t.Fatalf("move path = %s", moved.GetPath())
	}

	copied, err := d.Copy(ctx, moved, dir)
	if err != nil {
		t.Fatal(err)
	}
	if copied.GetPath() != "/pics/b.png" {
		t.Fatalf("copy path = %s", copied.GetPath())
	}

	if err = d.Remove(ctx, moved); err != nil {
		t.Fatal(err)
	}
	rootListed, err := d.List(ctx, rootDir(), model.ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rootListed) != 1 || rootListed[0].GetName() != "pics" {
		t.Fatalf("root after remove = %+v", rootListed)
	}
	if d.Index == "" {
		t.Fatal("index should persist locally after mutation")
	}
}
