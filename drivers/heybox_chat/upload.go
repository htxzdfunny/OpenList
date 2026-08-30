package heybox_chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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
)

func (d *HeyboxChat) queryParams(apiPath string) map[string]string {
	ts := time.Now().Unix()
	nonce := createNonce(ts)
	return map[string]string{
		"heybox_id":        d.HeyboxID,
		"pkey":             d.Pkey,
		"client_type":      "heybox_chat",
		"x_client_type":    "pc",
		"os_type":          "web",
		"x_os_type":        "Windows",
		"device_info":      "Chrome",
		"x_app":            "heybox_chat",
		"version":          "999.0.4",
		"web_version":      "1.0.0",
		"chat_os_type":     "client",
		"chat_version":     chatVersion,
		"chat_exe_version": chatVersion,
		"device_id":        d.deviceIDOrNew(),
		"_time":            strconv.FormatInt(ts, 10),
		"nonce":            nonce,
		"hkey":             createHkey(apiPath, ts, nonce),
	}
}

func (d *HeyboxChat) deviceIDOrNew() string {
	if d.deviceID == "" {
		d.deviceID = strings.ToUpper(uuid.NewString())
	}
	return d.deviceID
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
	if d.client == nil {
		d.client = base.NewRestyClient()
	}
}

func (d *HeyboxChat) request(ctx context.Context, apiPath string, form map[string]string, result any) error {
	d.ensureClient()
	var raw apiResponse
	resp, err := d.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/x-www-form-urlencoded;charset=utf-8").
		SetCookie(&http.Cookie{Name: "user_pkey", Value: d.Pkey}).
		SetQueryParams(d.queryParams(apiPath)).
		SetFormData(form).
		SetResult(&raw).
		Post(d.endpoint(apiPath))
	if err != nil {
		return err
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
		return fmt.Errorf("heybox api %s: %s", apiPath, msg)
	}
	if result != nil && len(raw.Result) > 0 && string(raw.Result) != "null" {
		return json.Unmarshal(raw.Result, result)
	}
	return nil
}

func (d *HeyboxChat) requestUploadInfo(ctx context.Context, info fileInfo) (*uploadInfoResult, error) {
	form, err := buildInfoForm(info)
	if err != nil {
		return nil, err
	}
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
