// blob：全局内容寻址实体层。全站同一 SHA256 只保留一份物理文件
// （<StoragePath>/blobs/<h[0:2]>/<h[2:4]>/<hash>），多个 File 索引（可跨用户、
// 含回收站）共享同一实体。
//
// 两个关键不变式：
//  1. 引用计数：Blob.RefCount == "引用该 hash 的 File 索引数"，归零即删除物理实体并摘除记录。
//     因此"用户删除只删除索引，不删除源文件"——只要还有任何索引（含他人、含回收站）引用，
//     物理文件就保留。
//  2. 去重后计费：Blob.BilledUserID 是该实体占用的配额归属用户（首个引用者计费）；
//     计费者索引全部消失而实体仍被他人引用时，计费自动转移给下一个引用者；归零则退还。
//
// 所有函数都接受 *gorm.DB（可以是调用方的事务），保证与索引行的增删同事务提交；
// ref_count 用 SQL 原子增减，Release 用行锁读取，避免并发归零竞态。
package blob

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// ErrQuotaExceeded 配额不足（调用方应回滚事务并返回 507）
var ErrQuotaExceeded = errors.New("存储空间不足")

// Path 返回实体物理路径
func Path(hash string) string { return util.BlobPath(hash) }

// Exists 实体是否已在磁盘上（用于秒传判断与写盘前去重）
func Exists(hash string) bool {
	if hash == "" {
		return false
	}
	fi, err := os.Stat(util.BlobPath(hash))
	return err == nil && !fi.IsDir()
}

// Size 实体大小；不存在返回 -1
func Size(hash string) int64 {
	if hash == "" {
		return -1
	}
	fi, err := os.Stat(util.BlobPath(hash))
	if err != nil || fi.IsDir() {
		return -1
	}
	return fi.Size()
}

// Get 读取实体登记信息（不存在返回 gorm.ErrRecordNotFound）
func Get(hash string) (*model.Blob, error) {
	var b model.Blob
	if err := db.DB.Where("hash = ?", hash).First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// Acquire 登记一次引用（在索引行插入的同一事务内调用）：
//   - 实体已在册：ref_count + 1，不改变计费归属（同一用户重复上传同一内容不会重复计费）；
//   - 实体新建：本用户成为计费者并计入配额；
//   - 实体在册但无人计费（billed_user_id = 0，上一任计费者已退出）：本用户接手计费。
//
// 返回 (当前计费用户, 本次是否向本用户收费, error)；配额不足返回 ErrQuotaExceeded。
func Acquire(tx *gorm.DB, userID uint, hash string, size int64) (uint, bool, error) {
	if hash == "" {
		return 0, false, errors.New("实体 hash 为空")
	}
	// 实体目录先行创建（物理写入由调用方负责，保证"实体文件先于索引行存在"）
	if err := util.EnsureDir(filepath.Dir(util.BlobPath(hash))); err != nil {
		return 0, false, err
	}
	res := tx.Exec(`INSERT INTO blobs (hash, size, ref_count, billed_user_id, created_at, updated_at)
		VALUES (?, ?, 1, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE ref_count = ref_count + 1, size = VALUES(size), updated_at = NOW()`,
		hash, size, userID)
	if res.Error != nil {
		return 0, false, res.Error
	}
	// MySQL：受影响行数 1=插入（首次），2=更新（已存在）
	first := res.RowsAffected == 1

	var b model.Blob
	if err := tx.Where("hash = ?", hash).First(&b).Error; err != nil {
		return 0, false, err
	}
	if !first && b.BilledUserID != 0 {
		return b.BilledUserID, false, nil
	}
	if err := charge(tx, userID, size, true); err != nil {
		return 0, false, err
	}
	if err := tx.Model(&model.Blob{}).Where("hash = ?", hash).
		UpdateColumn("billed_user_id", userID).Error; err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

// Release 释放一次引用。**调用方必须先删除（或已删除）对应 File 索引行**，
// 并把该索引原本指向的实体路径 path 传进来（用于未迁移存量行的兜底清理）。
//
// 行为：
//  1. 引用归零 → 退还计费者配额、删除物理实体、摘除 blob 记录（引用数归零自动回收）；
//  2. 计费者已无该实体的任何索引（含回收站）→ 计费转移给下一个引用者，无引用者则置 0 并退还；
//  3. 实体不在册（存量未迁移）→ 退化为"全站无同 hash 索引时删除该路径文件"的旧语义。
func Release(tx *gorm.DB, userID uint, hash string, size int64, path string) error {
	if hash == "" {
		return nil
	}
	var b model.Blob
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("hash = ?", hash).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return releaseLegacy(tx, userID, hash, size, path)
	}
	if err != nil {
		return err
	}

	b.RefCount--
	if b.RefCount <= 0 {
		refund(tx, b.BilledUserID, b.Size)
		if err := tx.Where("hash = ?", hash).Delete(&model.Blob{}).Error; err != nil {
			return err
		}
		p := util.BlobPath(hash)
		os.Remove(p)
		// 兼容未迁移的存量索引：旧路径上的实体一并清理
		if path != "" && path != p {
			os.Remove(path)
		}
		return nil
	}

	updates := map[string]any{"ref_count": b.RefCount}
	// 计费者退出（对我已无任何索引，含回收站）→ 转移计费权
	if b.BilledUserID == userID && !userHasHash(tx, userID, hash) {
		next := nextHolder(tx, hash)
		updates["billed_user_id"] = next
		refund(tx, userID, b.Size)
		if next != 0 {
			// 计费转移不做配额校验：实体已存在于磁盘，这里只是占用归属变更
			_ = charge(tx, next, b.Size, false)
		}
	}
	return tx.Model(&model.Blob{}).Where("hash = ?", hash).Updates(updates).Error
}

// releaseLegacy 存量兜底（该 hash 尚无 blob 记录，实体在旧路径下）
func releaseLegacy(tx *gorm.DB, userID uint, hash string, size int64, path string) error {
	var refs int64
	tx.Model(&model.File{}).Where("hash = ? AND type = 1", hash).Count(&refs)
	if refs == 0 && path != "" {
		os.Remove(path)
	}
	// 旧口径：索引删除即退还其所有者配额
	refund(tx, userID, size)
	return nil
}

// charge 增加配额；enforce=true 时做上限校验（超额返回 ErrQuotaExceeded），
// enforce=false 用于计费转移（实体已存在，不因超额而失败）。
func charge(tx *gorm.DB, userID uint, size int64, enforce bool) error {
	if size <= 0 || userID == 0 {
		return nil
	}
	q := tx.Model(&model.User{}).Where("id = ?", userID)
	if enforce {
		q = q.Where("quota_used + ? <= quota_max", size)
	}
	res := q.UpdateColumn("quota_used", gorm.Expr("quota_used + ?", size))
	if res.Error != nil {
		return res.Error
	}
	if enforce && res.RowsAffected == 0 {
		return ErrQuotaExceeded
	}
	return nil
}

// refund 退还配额（不会退到负数）
func refund(tx *gorm.DB, userID uint, size int64) {
	if size <= 0 || userID == 0 {
		return
	}
	tx.Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("quota_used", gorm.Expr("GREATEST(quota_used - ?, 0)", size))
}

// userHasHash 该用户是否还有指向此 hash 的索引（含回收站，回收站索引仍持引用）
func userHasHash(tx *gorm.DB, userID uint, hash string) bool {
	var n int64
	tx.Model(&model.File{}).Where("user_id = ? AND hash = ? AND type = 1", userID, hash).Count(&n)
	return n > 0
}

// nextHolder 下一个可接手的计费用户（按 user_id 升序 = 最早的引用者）
func nextHolder(tx *gorm.DB, hash string) uint {
	var row struct{ UserID uint }
	tx.Model(&model.File{}).Select("user_id").
		Where("hash = ? AND type = 1", hash).
		Group("user_id").Order("user_id ASC").Limit(1).Scan(&row)
	return row.UserID
}

// RefItem 一条索引（引用实体的具体文件记录）
type RefItem struct {
	FileID    uint   `json:"file_id"`
	Name      string `json:"name"`
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	Size      int64  `json:"size"`
	IsDeleted int    `json:"is_deleted"`
	CreatedAt string `json:"created_at"`
}

// Refs 返回某实体的全部索引（管理端展示用，含用户名与回收站状态）
func Refs(hash string) ([]RefItem, error) {
	var files []model.File
	if err := db.DB.Where("hash = ? AND type = 1", hash).Order("user_id ASC, id ASC").Find(&files).Error; err != nil {
		return nil, err
	}
	names := map[uint]string{}
	items := make([]RefItem, 0, len(files))
	for _, f := range files {
		uname, ok := names[f.UserID]
		if !ok {
			var u model.User
			if err := db.DB.First(&u, f.UserID).Error; err == nil {
				uname = u.Username
			}
			names[f.UserID] = uname
		}
		items = append(items, RefItem{
			FileID: f.ID, Name: f.Name, UserID: f.UserID, Username: uname,
			Size: f.Size, IsDeleted: f.IsDeleted,
			CreatedAt: f.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return items, nil
}

// StatsResult 存储索引聚合统计
type StatsResult struct {
	BlobCount      int64 `json:"blob_count"`       // 实体数量
	PhysicalBytes  int64 `json:"physical_bytes"`   // 物理占用（实体去重后真实占用）
	LogicalBytes   int64 `json:"logical_bytes"`    // 逻辑占用（全部文件索引大小之和）
	LegacyBytes    int64 `json:"legacy_bytes"`     // 尚未迁移到全局实体的旧数据字节数
	SavedBytes     int64 `json:"saved_bytes"`      // 去重节省（逻辑 - 物理 - 未迁移部分）
	TrashOnlyCount int64 `json:"trash_only_count"` // 仅被回收站索引引用的实体数
	UnbilledCount  int64 `json:"unbilled_count"`   // 无人计费的实体数（异常状态，可由校验修复）
	RefCountSum    int64 `json:"ref_count_sum"`    // 全部实体引用数之和
}

// Stats 聚合统计（管理端「存储索引」页顶部指标）
func Stats() (StatsResult, error) {
	var out StatsResult
	if err := db.DB.Model(&model.Blob{}).Count(&out.BlobCount).Error; err != nil {
		return out, err
	}
	var agg struct {
		Physical int64
		Refs     int64
	}
	db.DB.Model(&model.Blob{}).
		Select("COALESCE(SUM(size),0) AS physical, COALESCE(SUM(ref_count),0) AS refs").
		Scan(&agg)
	out.PhysicalBytes = agg.Physical
	out.RefCountSum = agg.Refs

	var logical struct{ Total int64 }
	db.DB.Raw("SELECT COALESCE(SUM(size),0) AS total FROM files WHERE type = 1").Scan(&logical)
	out.LogicalBytes = logical.Total

	// 未迁移：hash 尚无实体登记的索引大小之和
	var legacy struct{ Total int64 }
	db.DB.Raw(`SELECT COALESCE(SUM(size),0) AS total FROM files
		WHERE type = 1 AND hash <> '' AND hash NOT IN (SELECT hash FROM blobs)`).Scan(&legacy)
	out.LegacyBytes = legacy.Total

	out.SavedBytes = out.LogicalBytes - out.PhysicalBytes - out.LegacyBytes
	if out.SavedBytes < 0 {
		out.SavedBytes = 0
	}

	db.DB.Model(&model.Blob{}).Where("billed_user_id = 0").Count(&out.UnbilledCount)

	// 仅被回收站引用的实体：所有索引的 is_deleted 均为 1
	var trashOnly struct{ N int64 }
	db.DB.Raw(`SELECT COUNT(*) AS n FROM (
		SELECT hash FROM files WHERE type = 1 AND hash <> ''
		GROUP BY hash HAVING SUM(is_deleted = 0) = 0
	) t`).Scan(&trashOnly)
	out.TrashOnlyCount = trashOnly.N
	return out, nil
}

// QuotaChange 单个用户的配额变化（校验报告用）
type QuotaChange struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Before   int64  `json:"before"`
	After    int64  `json:"after"`
	Delta    int64  `json:"delta"`
}

// Report 校验/修复报告
type Report struct {
	Apply         bool          `json:"apply"`
	BlobMismatch  int64         `json:"blob_mismatch"`  // ref_count/billed_user_id 不一致的实体数
	RefFixed      int64         `json:"ref_fixed"`      // 修复的引用计数条数
	BilledFixed   int64         `json:"billed_fixed"`   // 修复的计费归属条数
	OrphanBlobs   int64         `json:"orphan_blobs"`   // 无索引引用却仍登记的实体数
	OrphanFiles   int64         `json:"orphan_files"`   // 磁盘上无索引引用的实体文件数
	Removed       int64         `json:"removed"`        // 实际删除的实体文件数
	QuotaChanged  []QuotaChange `json:"quota_changed"`  // 配额被修正的用户
	LegacyRows    int64         `json:"legacy_rows"`    // 尚未迁移的索引行数
	OrphanSamples []string      `json:"orphan_samples"` // 孤儿文件样例（最多 20 条）
}

// BlobItem 管理端实体列表项（附带计费用户名）
type BlobItem struct {
	Hash           string `json:"hash"`
	Size           int64  `json:"size"`
	RefCount       int64  `json:"ref_count"`
	BilledUserID   uint   `json:"billed_user_id"`
	BilledUsername string `json:"billed_username"`
	CreatedAt      string `json:"created_at"`
	OnDisk         bool   `json:"on_disk"`
}

// List 管理端实体列表（分页 + hash 关键字）
func List(page, pageSize int, keyword string) (int64, []BlobItem, error) {
	query := db.DB.Model(&model.Blob{})
	if keyword != "" {
		query = query.Where("hash LIKE ?", "%"+strings.ToLower(keyword)+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var blobs []model.Blob
	if err := query.Order("size DESC, hash ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&blobs).Error; err != nil {
		return 0, nil, err
	}
	names := map[uint]string{}
	items := make([]BlobItem, 0, len(blobs))
	for _, b := range blobs {
		uname := "—"
		if b.BilledUserID != 0 {
			if n, ok := names[b.BilledUserID]; ok {
				uname = n
			} else {
				var u model.User
				if err := db.DB.First(&u, b.BilledUserID).Error; err == nil {
					uname = u.Username
				}
				names[b.BilledUserID] = uname
			}
		}
		items = append(items, BlobItem{
			Hash: b.Hash, Size: b.Size, RefCount: b.RefCount,
			BilledUserID: b.BilledUserID, BilledUsername: uname,
			CreatedAt: b.CreatedAt.Format("2006-01-02 15:04:05"),
			OnDisk:    Exists(b.Hash),
		})
	}
	return total, items, nil
}

// groupRow 某 hash 的索引聚合（校验用）
type groupRow struct {
	Hash  string
	Refs  int64
	First uint
}

// Verify 校验并修复存储索引：
//   - dry-run：只统计差异，不做任何写操作；
//   - apply：重算 ref_count / billed_user_id、按计费关系重算 quota_used、
//     删除无引用的实体与其物理文件；gcOrphans 时额外清理磁盘上没有索引引用的实体文件。
func Verify(apply bool, gcOrphans bool) (Report, error) {
	rep := Report{Apply: apply, QuotaChanged: []QuotaChange{}, OrphanSamples: []string{}}
	if apply {
		err := db.DB.Transaction(func(tx *gorm.DB) error {
			rep = Report{Apply: true, QuotaChanged: []QuotaChange{}, OrphanSamples: []string{}}
			return collect(tx, &rep, true, gcOrphans)
		})
		return rep, err
	}
	err := collect(db.DB, &rep, false, gcOrphans)
	return rep, err
}

// collect 执行校验（apply=false 时只报告）
func collect(tx *gorm.DB, rep *Report, apply bool, gcOrphans bool) error {
	var groups []groupRow
	if err := tx.Raw(`SELECT hash, COUNT(*) AS refs, MIN(user_id) AS first
		FROM files WHERE type = 1 AND hash <> '' GROUP BY hash`).Scan(&groups).Error; err != nil {
		return err
	}
	var blobs []model.Blob
	if err := tx.Find(&blobs).Error; err != nil {
		return err
	}
	blobMap := make(map[string]model.Blob, len(blobs))
	for _, b := range blobs {
		blobMap[b.Hash] = b
	}
	groupMap := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupMap[g.Hash] = true
	}

	// ---- 1. 引用计数 / 计费归属 差异 ----
	for _, g := range groups {
		b, ok := blobMap[g.Hash]
		if !ok {
			// 尚未迁移的存量索引：单独计数（迁移脚本负责搬移，校验不改写）
			var legacy int64
			tx.Model(&model.File{}).Where("hash = ? AND type = 1", g.Hash).Count(&legacy)
			rep.LegacyRows += legacy
			continue
		}
		wantBilled := g.First
		// 计费者若仍是有效引用者则保留现状，仅当它已无任何索引时才纠正
		if b.BilledUserID != 0 && b.BilledUserID != g.First {
			var cnt int64
			tx.Model(&model.File{}).
				Where("hash = ? AND type = 1 AND user_id = ?", g.Hash, b.BilledUserID).Count(&cnt)
			if cnt > 0 {
				wantBilled = b.BilledUserID
			}
		}
		if b.RefCount != g.Refs || b.BilledUserID != wantBilled {
			rep.BlobMismatch++
			if b.RefCount != g.Refs {
				rep.RefFixed++
			}
			if b.BilledUserID != wantBilled {
				rep.BilledFixed++
			}
			if apply {
				if err := tx.Model(&model.Blob{}).Where("hash = ?", g.Hash).
					Updates(map[string]any{"ref_count": g.Refs, "billed_user_id": wantBilled}).Error; err != nil {
					return err
				}
			}
		}
	}

	// ---- 2. 无索引引用却仍登记的实体 ----
	for _, b := range blobs {
		if groupMap[b.Hash] {
			continue
		}
		rep.OrphanBlobs++
		if apply {
			if err := tx.Where("hash = ?", b.Hash).Delete(&model.Blob{}).Error; err != nil {
				return err
			}
			if err := os.Remove(util.BlobPath(b.Hash)); err == nil {
				rep.Removed++
			}
		}
	}

	// ---- 3. 磁盘孤儿：blobs/ 下无任何索引引用的实体文件 ----
	orphans, err := scanOrphanFiles(groupMap)
	if err != nil {
		return err
	}
	rep.OrphanFiles = int64(len(orphans))
	if apply && gcOrphans {
		for _, p := range orphans {
			if err := os.Remove(p); err == nil {
				rep.Removed++
			}
		}
		rep.OrphanFiles = 0
	}
	for i, p := range orphans {
		if i >= 20 {
			break
		}
		rep.OrphanSamples = append(rep.OrphanSamples, p)
	}

	// ---- 4. 按计费关系重算 quota_used ----
	quota := map[uint]int64{}
	for _, b := range blobs {
		if !groupMap[b.Hash] {
			continue // 已按孤儿处理
		}
		if b.BilledUserID != 0 {
			quota[b.BilledUserID] += b.Size
		}
	}
	// 未迁移部分按"索引各自计费"口径累加，与运行时兜底语义一致
	var legacy []struct {
		UserID uint
		Total  int64
	}
	if err := tx.Raw(`SELECT user_id, COALESCE(SUM(size),0) AS total FROM files
		WHERE type = 1 AND hash <> '' AND hash NOT IN (SELECT hash FROM blobs)
		GROUP BY user_id`).Scan(&legacy).Error; err != nil {
		return err
	}
	for _, l := range legacy {
		quota[l.UserID] += l.Total
	}

	var users []model.User
	if err := tx.Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		want := quota[u.ID]
		if want == u.QuotaUsed {
			continue
		}
		rep.QuotaChanged = append(rep.QuotaChanged, QuotaChange{
			UserID: u.ID, Username: u.Username, Before: u.QuotaUsed, After: want, Delta: want - u.QuotaUsed,
		})
		if apply {
			if err := tx.Model(&model.User{}).Where("id = ?", u.ID).
				UpdateColumn("quota_used", want).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// scanOrphanFiles 遍历 blobs/ 目录，返回无任何索引引用的实体文件路径
func scanOrphanFiles(known map[string]bool) ([]string, error) {
	orphans := []string{}
	err := filepath.WalkDir(util.BlobRoot(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 目录不存在/无权限时静默跳过
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		// 忽略合并中转文件
		if strings.HasSuffix(name, ".merge") || strings.HasSuffix(name, ".part") {
			return nil
		}
		if !known[name] {
			orphans = append(orphans, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return orphans, nil
}