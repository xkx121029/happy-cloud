package handler

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// BatchDownload 批量打包下载：将多个文件递归收集为 zip 流式输出
// 每个文件在 zip 内保留相对路径（文件夹层级）
func BatchDownload(c *gin.Context) {
	userID := uid(c)
	idsParam := c.Query("ids")
	if idsParam == "" {
		util.Fail(c, 400, "请指定要下载的文件 IDs")
		return
	}
	parts := strings.Split(idsParam, ",")
	if len(parts) == 0 {
		util.Fail(c, 400, "IDs 不能为空")
		return
	}

	// 收集所有合法文件 ID（按路径排序以确保 zip 内顺序一致）
	type fileEntry struct {
		id     uint
		name   string
		parent uint
		isDir  bool
		hash   string
		size   int64
	}
	var entries []fileEntry
	seen := make(map[uint]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range parts {
		id, _ := strconv.ParseUint(p, 10, 32)
		if id == 0 {
			continue
		}
		wg.Add(1)
		go func(uid uint) {
			defer wg.Done()
			var f model.File
			if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", uid, userID).First(&f).Error; err != nil {
				return
			}
			mu.Lock()
			if !seen[uid] {
				seen[uid] = true
				entries = append(entries, fileEntry{id: uid, name: f.Name, parent: f.ParentID, isDir: f.Type == 0, hash: f.Hash, size: f.Size})
			}
			mu.Unlock()
		}(uint(id))
	}
	wg.Wait()

	if len(entries) == 0 {
		util.Fail(c, 404, "未找到有效文件")
		return
	}

	// 建立 parent→children 索引（只包含本次选中的条目）
	childrenMap := make(map[uint][]fileEntry)
	for _, e := range entries {
		childrenMap[e.parent] = append(childrenMap[e.parent], e)
	}
	// 找根节点（parent 不在 entries 中，或 parent==0）
	rootIDs := make([]uint, 0)
	entryByID := make(map[uint]fileEntry, len(entries))
	for _, e := range entries {
		entryByID[e.id] = e
		if e.parent == 0 || !seen[e.parent] {
			rootIDs = append(rootIDs, e.id)
		}
	}

	// 生成 zip
	filename := fmt.Sprintf("download_%s.zip", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(filename)))
	c.Header("Content-Type", "application/zip")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

zw := zip.NewWriter(c.Writer)

	var collect func(parentID uint, path string) error
	collect = func(parentID uint, path string) error {
		for _, e := range childrenMap[parentID] {
			entryPath := filepath.Join(path, e.name)
			if e.isDir {
				// 写入目录条目
				h := &zip.FileHeader{
					Name:            entryPath + "/",
					Method:          zip.Store,
					UncompressedSize: 0,
				}
				h.SetModTime(time.Now())
				fw, err := zw.CreateHeader(h)
				if err != nil {
					return err
				}
				_ = fw
				if err := collect(e.id, entryPath); err != nil {
					return err
				}
			} else {
				// 写入文件
				h := &zip.FileHeader{
					Name:             entryPath,
					Method:           zip.Deflate,
					UncompressedSize: uint32(min(e.size, 0xFFFFFFFF)),
				}
				h.SetModTime(time.Now())
				fw, err := zw.CreateHeader(h)
				if err != nil {
					return err
				}
				// 从实体读取内容并写入 zip
				p := blobPathOfForHash(e.hash)
				if p == "" {
					continue // 实体缺失跳过
				}
				fr, err := os.Open(p)
				if err != nil {
					continue
				}
				if _, err := io.Copy(fw, fr); err != nil {
					fr.Close()
					return err
				}
				fr.Close()
			}
		}
		return nil
	}

	for _, rid := range rootIDs {
		e := entryByID[rid]
		if e.isDir {
			if err := collect(e.id, e.name); err != nil {
				zw.Close()
				util.Fail(c, 500, "打包失败")
				return
			}
		} else {
			// 单文件：直接写入 zip
			h := &zip.FileHeader{
				Name:             e.name,
				Method:           zip.Deflate,
				UncompressedSize: uint32(min(e.size, 0xFFFFFFFF)),
			}
			h.SetModTime(time.Now())
			fw, err := zw.CreateHeader(h)
			if err != nil {
				zw.Close()
				util.Fail(c, 500, "打包失败")
				return
			}
			p := blobPathOfForHash(e.hash)
			if p == "" {
				zw.Close()
				util.Fail(c, 404, "文件实体缺失")
				return
			}
			fr, err := os.Open(p)
			if err != nil {
				zw.Close()
				util.Fail(c, 500, "读取文件失败")
				return
			}
			if _, err := io.Copy(fw, fr); err != nil {
				fr.Close()
				zw.Close()
				util.Fail(c, 500, "打包失败")
				return
			}
			fr.Close()
		}
	}

	if err := zw.Close(); err != nil {
		util.Fail(c, 500, "写入 zip 失败")
		return
	}
	LogAction(userID, username(c), "batch_download", fmt.Sprintf("批量下载 %d 个文件", len(entries)))
	c.Status(http.StatusOK)
}

// blobPathOfForHash 根据 hash 获取实体路径（用于批量下载，不依赖 File 模型）
func blobPathOfForHash(hash string) string {
	if hash == "" {
		return ""
	}
	return util.BlobPath(hash)
}

// ListDownloadableFiles 列出用户所有可下载文件（供前端多选下载使用）
func ListDownloadableFiles(c *gin.Context) {
	userID := uid(c)
	var files []model.File
	db.DB.Where("user_id = ? AND type = 1 AND is_deleted = 0", userID).Order("name ASC").Find(&files)
	util.OK(c, files)
}
