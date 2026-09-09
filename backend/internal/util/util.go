package util

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/config"
)

// OK 统一成功响应 {code:0, message:"ok", data}
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

// Fail 统一失败响应，code 非 0
func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "message": msg, "data": nil})
}

// SHA256 计算内容哈希（用于秒传校验）
func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RandomToken 生成随机 token
func RandomToken(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// EnsureDir 确保目录存在
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// UserDir 用户文件存储目录
func UserDir(userID uint) string {
	return filepath.Join(config.Cfg.StoragePath, strconv.FormatUint(uint64(userID), 10))
}

// FilePath 文件实体路径
func FilePath(userID uint, hash string) string {
	return filepath.Join(UserDir(userID), hash)
}

// ChunkDir 分片临时目录
func ChunkDir(hash string) string {
	return filepath.Join(config.Cfg.StoragePath, "tmp", hash)
}
