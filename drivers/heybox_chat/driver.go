package heybox_chat

import (
	"context"
	"fmt"
	stdpath "path"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/go-resty/resty/v2"
)

type HeyboxChat struct {
	model.Storage
	Addition
	client   *resty.Client
	root     *Node
	mutex    sync.RWMutex
	apiHost  string
	deviceID string
}

func (d *HeyboxChat) Config() driver.Config {
	return config
}

func (d *HeyboxChat) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *HeyboxChat) Init(ctx context.Context) error {
	if strings.TrimSpace(d.HeyboxID) == "" || strings.TrimSpace(d.Pkey) == "" {
		return fmt.Errorf("heybox_id and pkey are required")
	}
	root, err := parseIndex(d.Index)
	if err != nil {
		return err
	}
	d.root = root
	return nil
}

func (d *HeyboxChat) Drop(ctx context.Context) error {
	return nil
}

func (d *HeyboxChat) Get(ctx context.Context, path string) (model.Obj, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := findNode(d.root, path)
	return nodeToObj(node, stdpath.Clean("/"+strings.TrimPrefix(path, "/")))
}

func (d *HeyboxChat) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := findNode(d.root, dir.GetPath())
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	if node.isFile() {
		return nil, errs.NotFolder
	}
	objs := make([]model.Obj, 0, len(node.Children))
	for _, child := range node.Children {
		obj, err := nodeToObj(child, stdpath.Join(dir.GetPath(), child.Name))
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func (d *HeyboxChat) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := findNode(d.root, file.GetPath())
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	if !node.isFile() {
		return nil, errs.NotFile
	}
	return &model.Link{URL: node.URL}, nil
}

func (d *HeyboxChat) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	parent := findNode(d.root, parentDir.GetPath())
	if parent == nil {
		return nil, errs.ObjectNotFound
	}
	dir := &Node{Name: dirName, Modified: time.Now().Unix()}
	if err := addChild(parent, dir); err != nil {
		return nil, err
	}
	if err := d.updateStorage(); err != nil {
		return nil, err
	}
	return nodeToObj(dir, stdpath.Join(parentDir.GetPath(), dirName))
}

func (d *HeyboxChat) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) &&
		(len(dstDir.GetPath()) == len(srcObj.GetPath()) || strings.HasPrefix(dstDir.GetPath()[len(srcObj.GetPath()):], "/")) {
		return nil, fmt.Errorf("cannot move parent dir to child")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	srcParent := findNode(d.root, stdpath.Dir(srcObj.GetPath()))
	dst := findNode(d.root, dstDir.GetPath())
	src := findNode(d.root, srcObj.GetPath())
	if srcParent == nil || dst == nil || src == nil {
		return nil, errs.ObjectNotFound
	}
	if err := removeChild(srcParent, src.Name); err != nil {
		return nil, err
	}
	if err := addChild(dst, src); err != nil {
		_ = addChild(srcParent, src)
		return nil, err
	}
	if err := d.updateStorage(); err != nil {
		return nil, err
	}
	return nodeToObj(src, stdpath.Join(dstDir.GetPath(), src.Name))
}

func (d *HeyboxChat) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	parent := findNode(d.root, stdpath.Dir(srcObj.GetPath()))
	src := findNode(d.root, srcObj.GetPath())
	if parent == nil || src == nil {
		return nil, errs.ObjectNotFound
	}
	if src.Name != newName && childByName(parent, newName) != nil {
		return nil, errs.ObjectAlreadyExists
	}
	src.Name = newName
	if err := d.updateStorage(); err != nil {
		return nil, err
	}
	return nodeToObj(src, stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName))
}

func (d *HeyboxChat) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) &&
		(len(dstDir.GetPath()) == len(srcObj.GetPath()) || strings.HasPrefix(dstDir.GetPath()[len(srcObj.GetPath()):], "/")) {
		return nil, fmt.Errorf("cannot copy parent dir to child")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	src := findNode(d.root, srcObj.GetPath())
	dst := findNode(d.root, dstDir.GetPath())
	if src == nil || dst == nil {
		return nil, errs.ObjectNotFound
	}
	copied := src.deepCopy()
	if err := addChild(dst, copied); err != nil {
		return nil, err
	}
	if err := d.updateStorage(); err != nil {
		return nil, err
	}
	return nodeToObj(copied, stdpath.Join(dstDir.GetPath(), copied.Name))
}

func (d *HeyboxChat) Remove(ctx context.Context, obj model.Obj) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	parent := findNode(d.root, stdpath.Dir(obj.GetPath()))
	if err := removeChild(parent, obj.GetName()); err != nil {
		return err
	}
	return d.updateStorage()
}

func (d *HeyboxChat) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if file.GetSize() > d.maxSize() {
		return nil, fmt.Errorf("file exceeds max size %d", d.maxSize())
	}
	d.mutex.RLock()
	parent := findNode(d.root, dstDir.GetPath())
	exists := parent != nil && childByName(parent, file.GetName()) != nil
	d.mutex.RUnlock()
	if parent == nil {
		return nil, errs.ObjectNotFound
	}
	if parent.isFile() {
		return nil, errs.NotFolder
	}
	if exists {
		return nil, errs.ObjectAlreadyExists
	}

	seeker, size, err := loadSeekableUpload(file, &up)
	if err != nil {
		return nil, err
	}
	if size > d.maxSize() {
		return nil, fmt.Errorf("file exceeds max size %d", d.maxSize())
	}

	publicURL, key, host, err := d.uploadFile(ctx, file.GetName(), file.GetMimetype(), size, seeker, up)
	if err != nil {
		return nil, err
	}
	node := &Node{
		Name:     file.GetName(),
		URL:      publicURL,
		Key:      key,
		Host:     host,
		Size:     size,
		Modified: time.Now().Unix(),
		Mime:     guessMime(file.GetName(), file.GetMimetype()),
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	parent = findNode(d.root, dstDir.GetPath())
	if err = addChild(parent, node); err != nil {
		return nil, err
	}
	if err = d.updateStorage(); err != nil {
		return nil, err
	}
	return nodeToObj(node, stdpath.Join(dstDir.GetPath(), node.Name))
}

var (
	_ driver.Driver       = (*HeyboxChat)(nil)
	_ driver.Getter       = (*HeyboxChat)(nil)
	_ driver.MkdirResult  = (*HeyboxChat)(nil)
	_ driver.MoveResult   = (*HeyboxChat)(nil)
	_ driver.RenameResult = (*HeyboxChat)(nil)
	_ driver.CopyResult   = (*HeyboxChat)(nil)
	_ driver.Remove       = (*HeyboxChat)(nil)
	_ driver.PutResult    = (*HeyboxChat)(nil)
)
