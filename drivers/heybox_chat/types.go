package heybox_chat

import "encoding/json"

type Node struct {
	Name     string  `json:"name"`
	URL      string  `json:"url,omitempty"`
	Key      string  `json:"key,omitempty"`
	Host     string  `json:"host,omitempty"`
	Size     int64   `json:"size,omitempty"`
	Modified int64   `json:"modified,omitempty"`
	Mime     string  `json:"mime,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

func (n *Node) isFile() bool {
	return n != nil && n.URL != ""
}

func (n *Node) deepCopy() *Node {
	if n == nil {
		return nil
	}
	cp := *n
	if len(n.Children) == 0 {
		cp.Children = nil
		return &cp
	}
	cp.Children = make([]*Node, len(n.Children))
	for i, child := range n.Children {
		cp.Children[i] = child.deepCopy()
	}
	return &cp
}

type fileInfo struct {
	MimeType string `json:"mimetype"`
	FSize    int64  `json:"fsize"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Ext      string `json:"ext,omitempty"`
}

type apiResponse struct {
	Status  string          `json:"status"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type uploadInfoResult struct {
	Bucket string   `json:"bucket"`
	Keys   []string `json:"keys"`
	Host   string   `json:"host"`
}

type uploadTokenResult struct {
	Credentials uploadCredentials `json:"credentials"`
	StartTime   int64             `json:"startTime"`
	ExpiredTime int64             `json:"expiredTime"`
}

type uploadCredentials struct {
	TmpSecretID  string `json:"tmpSecretId"`
	TmpSecretKey string `json:"tmpSecretKey"`
	SessionToken string `json:"sessionToken"`
}
