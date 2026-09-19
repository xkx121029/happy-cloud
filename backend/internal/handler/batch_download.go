package handler

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// BatchDownload 批量打包下载：将多个文件/文件夹递归收集为 zip 流式输出
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

	// 逐个根节点收集整棵子树（自动去重），保证选中文件夹时其全部后代一并打包
	entries := make([]zipEntry, 0)
	seen := make(map[uint]bool)
	for _, p := range parts {
		id, _ := strconv.ParseUint(p, 10, 32)
		if id == 0 {
			continue
		}
		sub, _ := collectSubtree(userID, uint(id))
		for _, e := range sub {
			if seen[e.id] {
				continue
			}
			seen[e.id] = true
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		util.Fail(c, 404, "未找到有效文件")
		return
	}

	filename := "download_" + time.Now().Format("20060102_150405")
	if err := writeZipEntries(c, filename, entries); err != nil {
		log.Printf("[BatchDownload] 打包失败: %v", err)
		util.Fail(c, 500, "打包失败")
		return
	}
	LogAction(userID, username(c), "batch_download", "批量下载")
}

// ListDownloadableFiles 列出用户所有可下载文件（供前端多选下载使用）
func ListDownloadableFiles(c *gin.Context) {
	userID := uid(c)
	var files []model.File
	db.DB.Where("user_id = ? AND type = 1 AND is_deleted = 0", userID).Order("name ASC").Find(&files)
	util.OK(c, files)
}
