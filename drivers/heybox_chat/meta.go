package heybox_chat

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	HeyboxID string `json:"heybox_id" required:"true" help:"heybox_id from a logged-in Heybox Chat client. Also sent as cookie user_heybox_id."`
	Pkey     string `json:"pkey" required:"true" secret:"true" help:"pkey from a logged-in Heybox Chat client. Sent as cookie user_pkey. Do not commit this value."`
	TokenID  string `json:"x_xhh_tokenid" secret:"true" help:"Optional. Cookie x_xhh_tokenid from api.xiaoheihe.cn. Fill this if user_pkey + user_heybox_id still return 非法的请求."`
	Index    string `json:"index" type:"text" help:"Local file index JSON. Updated after upload. Backup this field when migrating the storage."`
	Callback bool   `json:"callback" type:"bool" default:"false" help:"Call official callback/v2 after COS upload. Enable if CDN occasionally returns 404."`
	MaxSize  int64  `json:"max_size" type:"number" default:"10485760" help:"Max upload size in bytes. Official chat image limit is 10MB."`
}

func (Addition) GetRootPath() string {
	return "/"
}

var config = driver.Config{
	Name:        "HeyboxChat",
	LocalSort:   true,
	NoCache:     true,
	DefaultRoot: "/",
	CheckStatus: true,
	// OnlyIndices is UrlTree-only: op.Put replaces the stream with a nameless
	// FileStream (nil Reader). Heybox still needs the real image bytes.
	Alert: "warning|Unofficial Heybox Chat COS upload API. Files may be reviewed, expire, or rate-limited. Use your own login.",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &HeyboxChat{}
	})
}
