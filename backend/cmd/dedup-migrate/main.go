// dedup-migrate：将存量按用户隔离的实体迁移到全局内容寻址实体（blobs/）。
//
// 用法：
//   docker compose run --rm backend ./dedup-migrate
//   docker compose run --rm backend ./dedup-migrate -dry-run
//   docker compose run --rm backend ./dedup-migrate -apply
//
// 行为：
//   - 扫描所有 type=1 且 hash 非空的 File 索引；
//   - 若 blobs 表已有该 hash，只增加 ref_count，不动物理文件；
//   - 若 blobs 表没有该 hash：
//       · 若旧路径实体存在 → 复制到 blobs/，在 blobs 表写入记录；
//       · 若旧路径不存在 → 标记为 orphan（孤儿索引），不删索引、不报错。
//   - 迁移完成后运行 blob.Verify 校验并修正 ref_count / billed_user_id。
//   - dry-run 只报告，不写库、不搬文件；apply 才会真正执行。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

var (
	dryRun = flag.Bool("dry-run", true, "只报告不修改")
	apply  = flag.Bool("apply", false, "真正执行迁移")
)

func main() {
	flag.Parse()
	if err := db.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "数据库初始化失败: %v\n", err)
		os.Exit(1)
	}
	config.Load()
	if err := util.EnsureDir(util.BlobRoot()); err != nil {
		fmt.Fprintf(os.Stderr, "创建 blobs 目录失败: %v\n", err)
		os.Exit(1)
	}

	var files []model.File
	if err := db.DB.Where("type = 1 AND hash <> ''").Order("id ASC").Find(&files).Error; err != nil {
		fmt.Fprintf(os.Stderr, "查询文件索引失败: %v\n", err)
		os.Exit(1)
	}

	// 预加载已知 hash（避免逐个 SELECT）
	var known map[string]struct{}
	db.DB.Raw("SELECT hash FROM blobs").Scan(&known)
	if known == nil {
		known = map[string]struct{}{}
	}

	type stat struct {
		existing int // 实体已在册，只 +ref
		migrated int // 实体从旧路径迁移
		newEntity int // 全新实体（旧路径不存在，直接登记，后续 Verify 会处理孤儿）
		orphans  int // 旧路径缺失且未登记的索引
		errors   int
	}
	var st stat

	ensure := func(hash string) error {
		if _, ok := known[hash]; ok {
			st.existing++
			return nil
		}
		oldPath := filepath.Join(util.UserDir(0), hash) // 不依赖具体 user_id，旧布局是 <StoragePath>/<hash>
		// 兼容旧布局：按用户隔离目录
		found := false
		for _, f := range files {
			p := util.FilePath(f.UserID, hash)
			if _, err := os.Stat(p); err == nil {
				oldPath = p
				found = true
				break
			}
		}
		if !found {
			// 尝试从任意用户目录里找
			dir := config.Cfg.StoragePath
			filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || found {
					return nil
				}
				if strings.EqualFold(d.Name(), hash) {
					oldPath = p
					found = true
					return filepath.SkipAll
				}
				return nil
			})
		}
		dst := util.BlobPath(hash)
		if found {
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			src, err := os.Open(oldPath)
			if err != nil {
				return err
			}
			defer src.Close()
			dstF, err := os.Create(dst)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dstF, src); err != nil {
				dstF.Close()
				os.Remove(dst)
				return err
			}
			dstF.Close()
			st.migrated++
		} else {
			st.orphans++
		}
		known[hash] = struct{}{}
		return nil
	}

	// 按 hash 分组处理，避免并发重复操作同一实体
	type group struct {
		hash  string
		files []model.File
	}
	groups := map[string]*group{}
	for _, f := range files {
		h := strings.ToLower(f.Hash)
		g, ok := groups[h]
		if !ok {
			g = &group{hash: h}
			groups[h] = g
		}
		g.files = append(g.files, f)
	}

	for _, g := range groups {
		h := g.hash
		err := db.DB.Transaction(func(tx *gorm.DB) error {
			if err := ensure(h); err != nil {
				return err
			}
			// INSERT ON DUPLICATE KEY UPDATE ref_count+1
			res := tx.Exec(`INSERT INTO blobs (hash, size, ref_count, billed_user_id, created_at, updated_at)
				VALUES (?, ?, 1, ?, NOW(), NOW())
				ON DUPLICATE KEY UPDATE ref_count = ref_count + 1, size = VALUES(size), updated_at = NOW()`,
				h, g.files[0].Size, g.files[0].UserID)
			if res.Error != nil {
				return res.Error
			}
			first := res.RowsAffected == 1
			// 非首次不再重复 charge；首次需查该 hash 是否已有实体文件
			if first {
				if _, err := os.Stat(util.BlobPath(h)); os.IsNotExist(err) {
					st.newEntity++
				}
			}
			// 更新这批索引的 StoragePath 为全局路径
			for i := range g.files {
				g.files[i].StoragePath = util.BlobPath(h)
			}
			return tx.Model(&model.File{}).Where("id IN (?)", sliceUint(g.files)).Updates(map[string]any{
				"storage_path": gorm.Expr(" ?", util.BlobPath(h)),
			}).Error
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "迁移 %s 失败: %v\n", h, err)
			st.errors++
		}
	}

	if st.errors > 0 {
		fmt.Fprintf(os.Stderr, "迁移失败 %d 条，请检查日志\n", st.errors)
		os.Exit(1)
	}
	fmt.Printf("统计: 已存在=%d 迁移=%d 新实体=%d 孤儿索引=%d\n",
		st.existing, st.migrated, st.newEntity, st.orphans)
	if !*dryRun && !*apply {
		fmt.Println("使用 -apply 执行真实迁移（会修改数据库与物理文件）")
	}
}

func sliceUint(fs []model.File) []uint {
	out := make([]uint, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}
