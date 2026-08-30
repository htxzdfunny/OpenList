package heybox_chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	apiBase     = "https://api.xiaoheihe.cn"
	chatVersion = "1.56.0"
	cosRegion   = "ap-shanghai"
	cosEndpoint = "https://cos.ap-shanghai.myqcloud.com"
	infoAPI     = "/bbs/app/api/qcloud/cos/upload/info/v2"
	tokenAPI    = "/bbs/app/api/qcloud/cos/upload/token/v2"
	callbackAPI = "/bbs/app/api/qcloud/cos/upload/callback/v2"
	clientUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) heybox-chat-electron/1.52.0 Chrome/130.0.6723.118 Electron/33.2.0 Safari/537.36 BASE_API/https%3A%2F%2Fchat.xiaoheihe.cn BASE_HEYBOX_API/https%3A%2F%2Fapi.xiaoheihe.cn EXE_VERSION/1.56.0"
	// The api.xiaoheihe.cn gateway rejects requests without this Referer as
	// "非法的请求" even when signature, cookies and query are all valid.
	clientReferer = "https://chat.xiaoheihe.cn"
)

func (d *HeyboxChat) queryParams(apiPath string) map[string]string {
	ts := time.Now().Unix()
	nonce := createNonce(ts)
	return map[string]string{
		"heybox_id":         d.HeyboxID,
		"client_type":       "heybox_chat",
		"x_client_type":     "pc",
		"os_type":           "web",
		"x_os_type":         "Windows",
		"device_info":       "Chrome",
		"x_app":             "heybox_chat",
		"version":           "999.0.4",
		"web_version":       "1.0.0",
		"chat_os_type":      "client",
		"chat_version":      chatVersion,
		"chat_exe_version":  chatVersion,
		"electron_version":  "33.2.0",
		"client_bit":        "64",
		"win_version":       "10.0.19045",
		"device_id":         d.deviceIDOrNew(),
		"_time":             strconv.FormatInt(ts, 10),
		"nonce":             nonce,
		"hkey":              createHkey(apiPath, ts, nonce),
	}
}

func (d *HeyboxChat) deviceIDOrNew() string {
	if d.deviceID == "" {
		d.deviceID = strings.ToUpper(uuid.NewString())
	}
	return d.deviceID
}

func (d *HeyboxChat) cookieHeader() string {
	parts := []string{
		"user_pkey=" + strings.TrimSpace(d.Pkey),
		"user_heybox_id=" + strings.TrimSpace(d.HeyboxID),
	}
	if token := strings.TrimSpace(d.TokenID); token != "" {
		parts = append(parts, "x_xhh_tokenid="+token)
	}
	return strings.Join(parts, "; ")
}

func (d *HeyboxChat) endpoint(apiPath string) string {
	host := apiBase
	if d.apiHost != "" {
		host = strings.TrimRight(d.apiHost, "/")
	}
	return host + apiPath
}

func buildInfoForm(info fileInfo) (map[string]string, error) {
	fileInfos, err := marshalJSONString([]fileInfo{info})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"file_infos": fileInfos,
		"scope":      "chat",
		"type":       "",
		"source":     "",
		"need_cache": "0",
		"ext":        "",
	}, nil
}

func buildTokenForm(info *uploadInfoResult, mime string) (map[string]string, error) {
	keys, err := marshalJSONString(info.Keys)
	if err != nil {
		return nil, err
	}
	mimes, err := marshalJSONString([]string{mime})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"bucket":              info.Bucket,
		"keys":                keys,
		"mimetypes":           mimes,
		"is_multipart_upload": "0",
	}, nil
}

func buildCallbackForm(keys []string) (map[string]string, error) {
	keysStr, err := marshalJSONString(keys)
	if err != nil {
		return nil, err
	}
	return map[string]string{"keys": keysStr}, nil
}

func (d *HeyboxChat) ensureClient() {
	if d.client != nil {
		return
	}
	d.client = resty.New().
		SetTimeout(30 * time.Second).
		SetHeader("user-agent", clientUA).
		SetHeader("Accept", "application/json, text/plain, */*")
}

func encodeForm(form map[string]string) string {
	order := []string{"file_infos", "scope", "type", "source", "need_cache", "ext", "bucket", "keys", "mimetypes", "is_multipart_upload"}
	seen := make(map[string]bool, len(form))
	var b strings.Builder
	write := func(k, v string) {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(v))
	}
	for _, k := range order {
		if v, ok := form[k]; ok {
			write(k, v)
			seen[k] = true
		}
	}
	rest := make([]string, 0)
	for k := range form {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		write(k, form[k])
	}
	return b.String()
}

func redactPairs(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch strings.ToLower(k) {
		case "pkey", "user_pkey", "x_xhh_tokenid", "tmpsecretid", "tmpsecretkey", "sessiontoken":
			out[k] = "***"
		default:
			out[k] = v
		}
	}
	return out
}

func formatPairs(in map[string]string) string {
	if len(in) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(in[k])
	}
	return b.String()
}

func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (d *HeyboxChat) apiError(apiPath string, httpStatus int, query, form map[string]string, body []byte, cause string) error {
	safeQuery := formatPairs(redactPairs(query))
	safeForm := formatPairs(form)
	raw := clipText(string(body), 1500)
	if raw == "" {
		raw = "-"
	}
	log.WithFields(log.Fields{
		"api":    apiPath,
		"http":   httpStatus,
		"cause":  cause,
		"query":  safeQuery,
		"form":   safeForm,
		"body":   raw,
		"heybox": d.HeyboxID,
	}).Error("heybox chat api failed")
	return fmt.Errorf("heybox api %s: %s | http=%d query=%s form=%s body=%s", apiPath, cause, httpStatus, safeQuery, safeForm, raw)
}

func (d *HeyboxChat) request(ctx context.Context, apiPath string, form map[string]string, result any) error {
	d.ensureClient()
	query := d.queryParams(apiPath)
	var raw apiResponse
	resp, err := d.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Content-Type", "application/x-www-form-urlencoded;charset=utf-8").
		SetHeader("User-Agent", clientUA).
		SetHeader("Referer", clientReferer).
		SetHeader("Cookie", d.cookieHeader()).
		SetQueryParams(query).
		SetBody(encodeForm(form)).
		SetResult(&raw).
		Post(d.endpoint(apiPath))
	if err != nil {
		return d.apiError(apiPath, 0, query, form, nil, err.Error())
	}
	if raw.Status != "ok" {
		msg := raw.Msg
		if msg == "" {
			msg = raw.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(resp.Body()))
		}
		if msg == "" {
			msg = fmt.Sprintf("http %d", resp.StatusCode())
		}
		return d.apiError(apiPath, resp.StatusCode(), query, form, resp.Body(), msg)
	}
	if result != nil && len(raw.Result) > 0 && string(raw.Result) != "null" {
		if err := json.Unmarshal(raw.Result, result); err != nil {
			return d.apiError(apiPath, resp.StatusCode(), query, form, resp.Body(), "decode result: "+err.Error())
		}
	}
	return nil
}

func (d *HeyboxChat) requestUploadInfo(ctx context.Context, info fileInfo) (*uploadInfoResult, error) {
	form, err := buildInfoForm(info)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"mime":   info.MimeType,
		"ext":    info.Ext,
		"size":   info.FSize,
		"width":  info.Width,
		"height": info.Height,
	}).Info("heybox chat upload info")
	var out uploadInfoResult
	if err := d.request(ctx, infoAPI, form, &out); err != nil {
		return nil, err
	}
	if out.Bucket == "" || len(out.Keys) == 0 || out.Keys[0] == "" {
		return nil, fmt.Errorf("heybox api %s: empty bucket or keys", infoAPI)
	}
	return &out, nil
}

func (d *HeyboxChat) requestUploadToken(ctx context.Context, info *uploadInfoResult, mime string) (*uploadTokenResult, error) {
	form, err := buildTokenForm(info, mime)
	if err != nil {
		return nil, err
	}
	var out uploadTokenResult
	if err := d.request(ctx, tokenAPI, form, &out); err != nil {
		return nil, err
	}
	if out.Credentials.TmpSecretID == "" || out.Credentials.TmpSecretKey == "" || out.Credentials.SessionToken == "" {
		return nil, fmt.Errorf("heybox api %s: empty credentials", tokenAPI)
	}
	return &out, nil
}

func (d *HeyboxChat) requestCallback(ctx context.Context, keys []string) error {
	form, err := buildCallbackForm(keys)
	if err != nil {
		return err
	}
	return d.request(ctx, callbackAPI, form, nil)
}

func (d *HeyboxChat) putCOS(ctx context.Context, bucket, key, mime string, body io.Reader, size int64, cred uploadCredentials, up driver.UpdateProgress) error {
	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(cred.TmpSecretID, cred.TmpSecretKey, cred.SessionToken),
		Region:      aws.String(cosRegion),
		Endpoint:    aws.String(cosEndpoint),
	})
	if err != nil {
		return err
	}
	uploader := s3manager.NewUploader(sess)
	if size > s3manager.MaxUploadParts*s3manager.DefaultUploadPartSize {
		uploader.PartSize = size / (s3manager.MaxUploadParts - 1)
	}
	input := &s3manager.UploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(mime),
		Body: driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
			Reader: &driver.SimpleReaderWithSize{
				Reader: body,
				Size:   size,
			},
			UpdateProgress: up,
		}),
	}
	_, err = uploader.UploadWithContext(ctx, input)
	return err
}

func (d *HeyboxChat) uploadFile(ctx context.Context, name, mime string, size int64, body io.ReadSeeker, up driver.UpdateProgress) (publicURL, key, host string, err error) {
	info := buildFileInfo(name, mime, size, body)
	uploadInfo, err := d.requestUploadInfo(ctx, info)
	if err != nil {
		return "", "", "", err
	}
	token, err := d.requestUploadToken(ctx, uploadInfo, info.MimeType)
	if err != nil {
		return "", "", "", err
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		return "", "", "", err
	}
	if err = d.putCOS(ctx, uploadInfo.Bucket, uploadInfo.Keys[0], info.MimeType, body, size, token.Credentials, up); err != nil {
		return "", "", "", err
	}
	if d.Callback {
		if cbErr := d.requestCallback(ctx, uploadInfo.Keys); cbErr != nil {
			log.WithError(cbErr).Warn("heybox chat callback failed")
		}
	}
	return joinPublicURL(uploadInfo.Host, uploadInfo.Keys[0]), uploadInfo.Keys[0], uploadInfo.Host, nil
}
