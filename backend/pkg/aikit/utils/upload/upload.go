// Package upload 文件上传客户端，支持 COS/OSS 云存储，STS 临时凭证鉴权.
package upload

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	cos "github.com/tencentyun/cos-go-sdk-v5"
	"github.com/xxtea/xxtea-go/xxtea"
)

// Config 上传服务配置.
type Config struct {
	Addr     string `yaml:"addr"`      // 上传服务地址，如 http://upload.szwb.imgo.tv
	PlatName string `yaml:"plat_name"` // 业务方代号
	PlatKey  string `yaml:"plat_key"`  // 业务方秘钥
	Uploader string `yaml:"uploader"`  // 上传者标识
	UseCDN   int    `yaml:"use_cdn"`   // 是否使用 CDN 域名 1-是 0-否
}

// UploadResult 上传结果.
type UploadResult struct {
	KeyInfo *KeyInfo `json:"key_info"`
	URL     string   `json:"url"`
	FileID  int64    `json:"file_id"`
}

// BucketInfo 云存储桶信息.
type BucketInfo struct {
	CloudID    int    `json:"cloudId"`
	CloudName  string `json:"cloudName"`
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	BucketID   int    `json:"bucketId"`
	BucketName string `json:"bucketName"`
}

// StsToken STS 临时凭证.
type StsToken struct {
	AccessKeyID     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	SecurityToken   string `json:"securityToken"`
}

// KeyInfo 文件存储信息.
type KeyInfo struct {
	ID     int64  `json:"id"`
	IDStr  string `json:"idstr"`
	Key    string `json:"key"`
	SubFix string `json:"subfix"`
}

type stsTokenData struct {
	BucketInfo *BucketInfo `json:"bucketInfo"`
	StsToken   *StsToken   `json:"stsToken"`
	KeyInfo    *KeyInfo    `json:"keyInfo"`
}

type apiResp[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

// Uploader 文件上传客户端.
type Uploader struct {
	cfg    *Config
	client *http.Client
}

// New 创建 Uploader，必填字段为空时 panic.
// 参数：cfg - 上传服务配置，Addr/PlatName/PlatKey/Uploader 为必填.
// 返回值：uploader - 上传客户端实例.
func New(cfg *Config) *Uploader {
	if cfg.Addr == "" || cfg.PlatName == "" || cfg.PlatKey == "" || cfg.Uploader == "" {
		panic("upload: Addr, PlatName, PlatKey and Uploader are required")
	}
	return &Uploader{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

// Upload 上传文件，优先用 filePath，其次用 fileBytes + fileName.
// 参数：ctx - 上下文, filePath - 本地文件路径, fileBytes - 文件字节内容, fileName - 文件名.
// 返回值：result - 上传结果，包含 URL 和文件 ID, err - 失败时的错误.
func (u *Uploader) Upload(ctx context.Context, filePath string, fileBytes []byte, fileName string) (*UploadResult, error) {
	data, subfix, err := u.readInput(filePath, fileBytes, fileName)
	if err != nil {
		return nil, err
	}

	sts, err := u.genStsToken(ctx, data, subfix)
	if err != nil {
		return nil, fmt.Errorf("upload: genStsToken: %w", err)
	}

	if err := u.putToCloud(ctx, sts, data); err != nil {
		return nil, fmt.Errorf("upload: put to cloud: %w", err)
	}

	if err := u.setACL(ctx, sts.KeyInfo.ID, "public-read"); err != nil {
		return nil, fmt.Errorf("upload: setACL: %w", err)
	}

	fileURL, err := u.getURL(ctx, sts.KeyInfo.ID)
	if err != nil {
		return nil, fmt.Errorf("upload: getURL: %w", err)
	}

	return &UploadResult{
		KeyInfo: sts.KeyInfo,
		URL:     fileURL,
		FileID:  sts.KeyInfo.ID,
	}, nil
}

// GetURL 根据文件 ID 获取访问 URL.
// 参数：ctx - 上下文, fileID - 文件 ID.
// 返回值：url - 文件访问地址, err - 失败时的错误.
func (u *Uploader) GetURL(ctx context.Context, fileID int64) (string, error) {
	return u.getURL(ctx, fileID)
}

// SetACL 设置文件权限.
// 参数：ctx - 上下文, fileID - 文件 ID, acl - 权限策略，如 "public-read".
// 返回值：err - 失败时的错误.
func (u *Uploader) SetACL(ctx context.Context, fileID int64, acl string) error {
	return u.setACL(ctx, fileID, acl)
}

// ---- internal ----.

func (u *Uploader) readInput(filePath string, fileBytes []byte, fileName string) ([]byte, string, error) {
	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			return nil, "", err
		}
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}
		return b, extOf(fileName, b), nil
	}
	if fileBytes != nil {
		return fileBytes, extOf(fileName, fileBytes), nil
	}
	return nil, "", fmt.Errorf("upload: filePath or fileBytes required")
}

func (u *Uploader) genStsToken(ctx context.Context, data []byte, subfix string) (*stsTokenData, error) {
	params := url.Values{}
	params.Set("platName", u.cfg.PlatName)
	params.Set("uploader", u.cfg.Uploader)
	params.Set("sign", u.sign())
	params.Set("fileHeadBase64", base64.StdEncoding.EncodeToString(data[:min(64, len(data))]))
	params.Set("fileSize", strconv.FormatInt(int64(len(data)), 10))
	params.Set("fileSubfix", subfix)
	params.Set("fileContentMd5", calcMD5(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.Addr+"/cloud/genStsToken", bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result apiResp[*stsTokenData]
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("genStsToken error code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data, nil
}

func (u *Uploader) putToCloud(ctx context.Context, sts *stsTokenData, data []byte) error {
	switch sts.BucketInfo.CloudID {
	case 22: // 腾讯云 COS
		return u.putCOS(ctx, sts, data)
	case 21: // 阿里云 OSS
		return u.putOSS(ctx, sts, data)
	default:
		return fmt.Errorf("unsupported cloudId=%d", sts.BucketInfo.CloudID)
	}
}

func (u *Uploader) putCOS(ctx context.Context, sts *stsTokenData, data []byte) error {
	endpoint, err := url.Parse(sts.BucketInfo.Endpoint)
	if err != nil {
		return err
	}
	endpoint.Host = sts.BucketInfo.BucketName + "." + endpoint.Host

	client := cos.NewClient(&cos.BaseURL{BucketURL: endpoint}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:     sts.StsToken.AccessKeyID,
			SecretKey:    sts.StsToken.AccessKeySecret,
			SessionToken: sts.StsToken.SecurityToken,
		},
	})
	_, err = client.Object.Put(ctx, sts.KeyInfo.Key, bytes.NewReader(data), nil)
	return err
}

func (u *Uploader) putOSS(ctx context.Context, sts *stsTokenData, data []byte) error {
	ossURL := fmt.Sprintf("%s/%s/%s", sts.BucketInfo.Endpoint, sts.BucketInfo.BucketName, sts.KeyInfo.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, ossURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("x-oss-security-token", sts.StsToken.SecurityToken)
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString([]byte(calcMD5(data))))
	// 必须使用 OSS V4 签名，V1/V2 已被部分 Region 禁用
	if err := signOSSV4(req, sts.StsToken.AccessKeyID, sts.StsToken.AccessKeySecret, sts.BucketInfo.BucketName, sts.KeyInfo.Key); err != nil {
		return err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("OSS PUT failed: %s", resp.Status)
	}
	return nil
}

func (u *Uploader) setACL(ctx context.Context, fileID int64, acl string) error {
	params := url.Values{}
	params.Set("platName", u.cfg.PlatName)
	params.Set("uploader", u.cfg.Uploader)
	params.Set("sign", u.sign())
	params.Set("id", strconv.FormatInt(fileID, 10))
	params.Set("acl", acl)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.Addr+"/acl/setObjectAcl", bytes.NewBufferString(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (u *Uploader) getURL(ctx context.Context, fileID int64) (string, error) {
	params := url.Values{}
	params.Set("platName", u.cfg.PlatName)
	params.Set("uploader", u.cfg.Uploader)
	params.Set("sign", u.sign())
	params.Set("id", strconv.FormatInt(fileID, 10))
	params.Set("useCdn", strconv.Itoa(u.cfg.UseCDN))
	params.Set("needResize", "0")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.Addr+"/download/getUrlById?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result apiResp[map[string]string]
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Data["url"], nil
}

func (u *Uploader) sign() string {
	raw := fmt.Sprintf("mgtv_str_sign(time=%d);", time.Now().Unix())
	key := fmt.Sprintf("%s&&%s", u.cfg.PlatKey, u.cfg.Uploader)
	return base64.StdEncoding.EncodeToString(xxtea.Encrypt([]byte(raw), []byte(key)))
}

// ---- helpers ----.

func calcMD5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// CalcCRC64 计算数据的 CRC64 校验值.
// 参数：data - 待校验数据.
// 返回值：str - 十进制 CRC64 校验字符串.
func CalcCRC64(data []byte) string {
	tbl := crc64.MakeTable(crc64.ECMA)
	return strconv.FormatUint(crc64.Checksum(data, tbl), 10)
}

// DetectExt 根据文件内容检测扩展名.
// 参数：data - 文件字节内容.
// 返回值：ext - 文件扩展名，无法识别时返回 "unknown".
func DetectExt(data []byte) string {
	if len(data) == 0 {
		return "unknown"
	}
	if bytes.HasPrefix(data, []byte("BM")) {
		return "bmp"
	}
	if len(data) < 4 {
		return "unknown"
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return "jpg"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && string(data[8:12]) == "WAVE":
		return "wav"
	case bytes.HasPrefix(data, []byte("ID3")) || bytes.HasPrefix(data, []byte("\xff\xfb")):
		return "mp3"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && string(data[8:12]) == "WEBP":
		return "webp"
	}

	if ext := strings.TrimPrefix(mimetype.Detect(data).Extension(), "."); ext != "" {
		switch ext {
		case "jpeg":
			return "jpg"
		case "plain": // 不能区分markdown、json等文本文件格式
			return "txt"
		default:
			return ext
		}
	}
	return "unknown"
}

func extOf(fileName string, data []byte) string {
	if fileName != "" {
		if ext := filepath.Ext(fileName); len(ext) > 1 {
			return ext[1:]
		}
	}
	return DetectExt(data)
}
