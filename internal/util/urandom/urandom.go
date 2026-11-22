package urandom

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"time"
)

// Hex 生成 nBytes 字节的加密随机十六进制字符串（长度为 2*nBytes）
func Hex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}

// ID 生成 16 位十六进制随机唯一标识（8 字节随机熵）
func ID() string {
	return Hex(8)
}

// String 从指定字符集 alphabet 中随机生成长度为 length 的字符串
func String(alphabet string, length int) (string, error) {
	alphabetLen := big.NewInt(int64(len(alphabet)))
	buf := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf), nil
}
