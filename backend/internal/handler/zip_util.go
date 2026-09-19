package handler

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// zipEntry 参与打包的一个节点
type zipEntry struct {
	id     uint
	name   string
	parent uint
	isDir  bool
	hash   string
	size   int64
}

// collectSubtree 收集 rootID（含自身）的整棵未删除子树，全部节点必须归属 ownerID。
// 供「文件夹下载/分享文件夹打包」复用：逐层 BFS，按 type,name 排序保证 zip 内顺序稳定。
func collectSubtree(ownerID, rootID uint) ([]zipEntry, error) {
	var entries []zipEntry
	seen := map[uint]bool{}
	queue := []uint{rootID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", id, ownerID).First(&f).Error; err != nil {
			continue // 不存在/已删除/非本人 → 跳过
		}
		entries = append(entries, zipEntry{id: f.ID, name: f.Name, parent: f.ParentID, isDir: f.Type == 0, hash: f.Hash, size: f.Size})
		if f.Type == 0 {
			var children []model.File
			if err := db.DB.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", ownerID, f.ID).
				Order("type DESC, name ASC").Find(&children).Error; err == nil {
				for _, ch := range children {
					queue = append(queue, ch.ID)
				}
			}
		}
	}
	return entries, nil
}

// writeZipEntries 将 entries（含父子层级）按相对路径流式写入 zip。
// 通用错误返回 error，调用方负责决定是否回滚输出。
func writeZipEntries(c *gin.Context, filename string, entries []zipEntry) error {
	if len(entries) == 0 {
		return nil
	}
	childrenMap := make(map[uint][]zipEntry)
	entryByID := make(map[uint]zipEntry, len(entries))
	for _, e := range entries {
		childrenMap[e.parent] = append(childrenMap[e.parent], e)
		entryByID[e.id] = e
	}
	// 根节点：parent==0 或父级不在本次集合中（用户从中间层选择）
	rootIDs := make([]uint, 0)
	for _, e := range entries {
		if e.parent == 0 {
			rootIDs = append(rootIDs, e.id)
			continue
		}
		if _, ok := entryByID[e.parent]; !ok {
			rootIDs = append(rootIDs, e.id)
		}
	}

	filename = fmt.Sprintf("%s.zip", trimExt(filename))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(filename)))
	c.Header("Content-Type", "application/zip")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	zw := zip.NewWriter(c.Writer)
	var collect func(parentID uint, path string) error
	collect = func(parentID uint, path string) error {
		for _, e := range childrenMap[parentID] {
			entryPath := filepath.Join(path, e.name)
			if e.isDir {
				h := &zip.FileHeader{Name: entryPath + "/", Method: zip.Store}
				h.SetModTime(time.Now())
				if _, err := zw.CreateHeader(h); err != nil {
					return err
				}
				if err := collect(e.id, entryPath); err != nil {
					return err
				}
				continue
			}
			h := &zip.FileHeader{Name: entryPath, Method: zip.Deflate, UncompressedSize: uint32(min(e.size, 0xFFFFFFFF))}
			h.SetModTime(time.Now())
			fw, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			p := blobPathOfForHash(e.hash)
			if p == "" {
				log.Printf("[zip] 文件 #%d %s 实体缺失（hash=%s），已跳过", e.id, e.name, e.hash)
				continue
			}
			fr, err := os.Open(p)
			if err != nil {
				log.Printf("[zip] 打开文件 #%d %s 失败: %v，已跳过", e.id, e.name, err)
				continue
			}
			if _, err := io.Copy(fw, fr); err != nil {
				fr.Close()
				return err
			}
			fr.Close()
		}
		return nil
	}

	for _, rid := range rootIDs {
		e := entryByID[rid]
		if err := collect(e.id, e.name); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return nil
}

// trimExt 去掉文件名已有扩展名，统一补 .zip（如 "照片.zip" → "照片"；"a.jpg" → "a"）
func trimExt(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return name[:len(name)-len(ext)]
}

// blobPathOfForHash 根据 hash 获取实体路径（不依赖 File 模型）
func blobPathOfForHash(hash string) string {
	if hash == "" {
		return ""
	}
	return util.BlobPath(hash)
}
