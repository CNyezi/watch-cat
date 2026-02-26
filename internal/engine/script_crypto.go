package engine

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// CryptoModule 提供 JS 脚本中的 crypto.xxx() 加密函数。
type CryptoModule struct{}

func (c *CryptoModule) Sha1(s string) string {
	h := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

func (c *CryptoModule) Md5(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

func (c *CryptoModule) Sha256(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func (c *CryptoModule) HmacSha256(s, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(s))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func (c *CryptoModule) Base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func (c *CryptoModule) Base64Decode(s string) string {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(data)
}
