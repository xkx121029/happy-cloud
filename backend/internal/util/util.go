package util

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/model"
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

// UserDir 用户文件存储目录（仅存量数据与迁移工具使用，新代码请用 BlobPath）
func UserDir(userID uint) string {
	return filepath.Join(config.Cfg.StoragePath, strconv.FormatUint(uint64(userID), 10))
}

// FilePath 【已废弃】旧的按用户隔离实体路径 <StoragePath>/<userID>/<hash>。
// 该布局导致同一文件按用户各存一份、无法跨用户去重，已被 BlobPath 取代；
// 仅存量迁移工具读取历史实体时使用。
func FilePath(userID uint, hash string) string {
	return filepath.Join(UserDir(userID), hash)
}

// BlobPath 全局内容寻址实体路径：<StoragePath>/blobs/<h[0:2]>/<h[2:4]>/<hash>。
// 两级散列目录避免单目录堆积海量文件（同一文件全站只存一份）。
func BlobPath(hash string) string {
	h := strings.ToLower(hash)
	if len(h) < 4 {
		return filepath.Join(config.Cfg.StoragePath, "blobs", h)
	}
	return filepath.Join(config.Cfg.StoragePath, "blobs", h[:2], h[2:4], h)
}

// BlobRoot 全局实体根目录 <StoragePath>/blobs，供孤儿扫描/迁移遍历使用
func BlobRoot() string {
	return filepath.Join(config.Cfg.StoragePath, "blobs")
}

// BlobPathOf 返回 File 索引指向的实体路径（优先全局 blob 路径，回退旧用户路径）
func BlobPathOf(f *model.File) string {
	if f.Hash != "" {
		if p := BlobPath(f.Hash); fileExists(p) {
			return p
		}
	}
	if f.StoragePath != "" && fileExists(f.StoragePath) {
		return f.StoragePath
	}
	return BlobPath(f.Hash)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// ChunkDir 分片临时目录（按用户隔离：tmp/<userID>/<hash>，
// 避免不同用户上传相同 hash 的分片互相覆盖/混用导致文件损坏，S6）
func ChunkDir(userID uint, hash string) string {
	return filepath.Join(config.Cfg.StoragePath, "tmp", strconv.FormatUint(uint64(userID), 10), hash)
}
