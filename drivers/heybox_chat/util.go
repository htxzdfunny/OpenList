package heybox_chat

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

func joinPublicURL(host, key string) string {
	host = strings.TrimSpace(host)
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if host == "" {
		return key
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	return strings.TrimRight(host, "/") + "/" + key
}

func fileExt(name string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
}

func normalizeExt(name string) string {
	ext := fileExt(name)
	if ext == "jpeg" {
		return "jpg"
	}
	return ext
}

func marshalJSONString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseIndex(text string) (*Node, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return &Node{Name: "root"}, nil
	}
	var root Node
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, fmt.Errorf("invalid index json: %w", err)
	}
	if root.Name == "" {
		root.Name = "root"
	}
	return &root, nil
}

func stringifyIndex(root *Node) (string, error) {
	if root == nil {
		root = &Node{Name: "root"}
	}
	b, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func splitVirtualPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

func findNode(root *Node, p string) *Node {
	if root == nil {
		return nil
	}
	cur := root
	for _, part := range splitVirtualPath(p) {
		cur = childByName(cur, part)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func childByName(parent *Node, name string) *Node {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

func addChild(parent *Node, child *Node) error {
	if parent == nil {
		return errs.ObjectNotFound
	}
	if parent.isFile() {
		return errs.NotFolder
	}
	if child == nil || child.Name == "" {
		return fmt.Errorf("invalid node")
	}
	if childByName(parent, child.Name) != nil {
		return errs.ObjectAlreadyExists
	}
	parent.Children = append(parent.Children, child)
	return nil
}

func removeChild(parent *Node, name string) error {
	if parent == nil {
		return errs.ObjectNotFound
	}
	for i, child := range parent.Children {
		if child.Name == name {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return nil
		}
	}
	return errs.ObjectNotFound
}

func nodeToObj(node *Node, p string) (model.Obj, error) {
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	modified := time.Time{}
	if node.Modified > 0 {
		modified = time.Unix(node.Modified, 0)
	}
	return &model.Object{
		Name:     node.Name,
		Size:     node.Size,
		Modified: modified,
		IsFolder: !node.isFile(),
		Path:     p,
	}, nil
}

func guessMime(name, mime string) string {
	if mime != "" && mime != "application/octet-stream" {
		return mime
	}
	switch normalizeExt(name) {
	case "png":
		return "image/png"
	case "jpg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		if mime != "" {
			return mime
		}
		return "application/octet-stream"
	}
}

func readImageSize(r io.ReadSeeker) (int, int, bool) {
	if r == nil {
		return 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(r)
	_, _ = r.Seek(0, io.SeekStart)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

func buildFileInfo(name, mime string, size int64, r io.ReadSeeker) fileInfo {
	mime = guessMime(name, mime)
	info := fileInfo{MimeType: mime, FSize: size, Ext: fileExt(name)}
	if strings.HasPrefix(mime, "image/") {
		if w, h, ok := readImageSize(r); ok {
			info.Width = w
			info.Height = h
		}
	}
	return info
}

func (d *HeyboxChat) maxSize() int64 {
	if d.MaxSize <= 0 {
		return 10 * 1024 * 1024
	}
	return d.MaxSize
}

func (d *HeyboxChat) updateStorage() error {
	text, err := stringifyIndex(d.root)
	if err != nil {
		return err
	}
	d.Index = text
	if d.ID == 0 {
		return nil
	}
	op.MustSaveDriverStorage(d)
	return nil
}
