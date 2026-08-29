package heybox_chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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

func (d *HeyboxChat) queryParams() map[string]string {
	return map[string]string{
		"heybox_id":     d.HeyboxID,
		"pkey":          d.Pkey,
		"client_type":   "heybox_chat",
		"x_client_type": "pc",
		"os_type":       "web",
		"x_app":         "heybox_chat",
		"version":       "999.0.4",
		"web_version":   "1.0.0",
		"chat_os_type":  "client",
		"chat_version":  chatVersion,
	}
}

func (d *HeyboxChat) ensureClient() {
	if d.client == nil {
		d.client = base.NewRestyClient()
	}
}

func (d *HeyboxChat) request(ctx context.Context, apiPath string, body any, result any) error {
	d.ensureClient()
	var raw apiResponse
	resp, err := d.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json;charset=utf-8").
		SetQueryParams(d.queryParams()).
		SetBody(body).
		SetResult(&raw).
		Post(apiBase + apiPath)
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

func (d *HeyboxChat) requestUploadInfo(ctx context.Context, info fileInfo, ext string) (*uploadInfoResult, error) {
	fileInfos, err := marshalJSONString([]fileInfo{info})
	if err != nil {
		return nil, err
	}
	body := base.Json{
		"file_infos": fileInfos,
		"scope":      "chat",
		"need_cache": 0,
	}
	if ext != "" {
		body["ext"] = ext
	}
	var out uploadInfoResult
	if err := d.request(ctx, infoAPI, body, &out); err != nil {
		return nil, err
	}
	if out.Bucket == "" || len(out.Keys) == 0 || out.Keys[0] == "" {
		return nil, fmt.Errorf("heybox api %s: empty bucket or keys", infoAPI)
	}
	return &out, nil
}

func (d *HeyboxChat) requestUploadToken(ctx context.Context, info *uploadInfoResult, mime string) (*uploadTokenResult, error) {
	keys, err := marshalJSONString(info.Keys)
	if err != nil {
		return nil, err
	}
	mimes, err := marshalJSONString([]string{mime})
	if err != nil {
		return nil, err
	}
	var out uploadTokenResult
	if err := d.request(ctx, tokenAPI, base.Json{
		"bucket":              info.Bucket,
		"keys":                keys,
		"mimetypes":           mimes,
		"is_multipart_upload": 0,
	}, &out); err != nil {
		return nil, err
	}
	if out.Credentials.TmpSecretID == "" || out.Credentials.TmpSecretKey == "" || out.Credentials.SessionToken == "" {
		return nil, fmt.Errorf("heybox api %s: empty credentials", tokenAPI)
	}
	return &out, nil
}

func (d *HeyboxChat) requestCallback(ctx context.Context, keys []string) error {
	keysStr, err := marshalJSONString(keys)
	if err != nil {
		return err
	}
	return d.request(ctx, callbackAPI, base.Json{"keys": keysStr}, nil)
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
	uploadInfo, err := d.requestUploadInfo(ctx, info, normalizeExt(name))
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
