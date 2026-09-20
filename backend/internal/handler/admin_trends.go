package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// Trends 数据趋势：返回最近 days 天每日注册/上传/上传字节数，用于管理面板图表
func Trends(c *gin.Context) {
	days := 30
	if v := c.Query("days"); v != "" {
		if n := atoiSafe(v); n >= 1 && n <= 90 {
			days = n
		}
	}

	// 生成日期骨架（升序），首日零点作为过滤起点
	start := time.Now().AddDate(0, 0, -days+1)
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
	dates := make([]string, 0, days)
	dayIdx := make(map[string]int, days)
	for i := 0; i < days; i++ {
		d := startDay.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		dates = append(dates, key)
		dayIdx[key] = i
	}
	registrations := make([]int, days)
	uploads := make([]int, days)
	uploadBytes := make([]int64, days)

	type row struct {
		Day  string
		Cnt  int
		Size int64
	}

	// 每日注册
	var regRows []row
	db.DB.Model(&model.User{}).
		Where("created_at >= ?", startDay).
		Select("DATE(created_at) AS day, COUNT(*) AS cnt").
		Group("DATE(created_at)").Scan(&regRows)
	for _, r := range regRows {
		if i, ok := dayIdx[r.Day]; ok {
			registrations[i] = r.Cnt
		}
	}

	// 每日上传文件数 + 字节数
	var upRows []row
	db.DB.Model(&model.File{}).
		Where("type = 1 AND created_at >= ?", startDay).
		Select("DATE(created_at) AS day, COUNT(*) AS cnt, COALESCE(SUM(size),0) AS size").
		Group("DATE(created_at)").Scan(&upRows)
	for _, r := range upRows {
		if i, ok := dayIdx[r.Day]; ok {
			uploads[i] = r.Cnt
			uploadBytes[i] = r.Size
		}
	}

	util.OK(c, gin.H{
		"days":          dates,
		"registrations": registrations,
		"uploads":       uploads,
		"upload_bytes":  uploadBytes,
	})
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
