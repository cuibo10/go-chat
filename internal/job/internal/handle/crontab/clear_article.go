package crontab

import (
	"go-chat/internal/dao/note"
	"go-chat/internal/entity"
	"go-chat/internal/model"
	"go-chat/internal/pkg/filesystem"
	"time"
)

type ClearArticle struct {
	articleAnnexDao *note.ArticleAnnexDao
	fileSystem      *filesystem.Filesystem
}

func NewClearArticle(articleAnnexDao *note.ArticleAnnexDao, fileSystem *filesystem.Filesystem) *ClearArticle {
	return &ClearArticle{articleAnnexDao: articleAnnexDao, fileSystem: fileSystem}
}

func (c *ClearArticle) Handle() error {

	c.clearArticleAnnex()

	c.clear()

	return nil
}

// 删除回收站文章附件
func (c *ClearArticle) clearArticleAnnex() {
	Db := c.articleAnnexDao.Db()

	lastId := 0
	size := 100

	for {
		items := make([]*model.ArticleAnnex, 0)

		err := Db.Model(&model.ArticleAnnex{}).Where("id > ? and status = 2 and deleted_at <= ?", lastId, time.Now().AddDate(0, 0, -30)).Order("id asc").Limit(size).Scan(&items).Error
		if err != nil {
			break
		}

		for _, item := range items {
			if item.Drive == entity.FileDriveLocal {
				_ = c.fileSystem.Local.Delete(item.Path)
			} else if item.Drive == entity.FileDriveCos {
				_ = c.fileSystem.Cos.Delete(item.Path)
			}

			Db.Delete(&model.ArticleAnnex{}, item.Id)
		}

		if len(items) < size {
			break
		}

		lastId = items[size-1].Id
	}
}

// 删除回收站笔记
func (c *ClearArticle) clear() {
	Db := c.articleAnnexDao.Db()

	lastId := 0
	size := 100

	for {
		items := make([]*model.Article, 0)

		err := Db.Model(&model.Article{}).Where("id > ? and status = 2 and deleted_at <= ?", lastId, time.Now().AddDate(0, 0, -30)).Order("id asc").Limit(size).Scan(&items).Error
		if err != nil {
			break
		}

		for _, item := range items {
			subItems := make([]*model.ArticleAnnex, 0)

			if err := Db.Model(&model.ArticleAnnex{}).Select("drive", "path").Where("article_id = ?", item.Id).Scan(&subItems).Error; err != nil {
				continue
			}

			for _, subItem := range subItems {
				if subItem.Drive == entity.FileDriveLocal {
					_ = c.fileSystem.Local.Delete(subItem.Path)
				} else if subItem.Drive == entity.FileDriveCos {
					_ = c.fileSystem.Cos.Delete(subItem.Path)
				}

				Db.Delete(&model.ArticleAnnex{}, subItem.Id)
			}

			Db.Delete(&model.Article{}, item.Id)
			Db.Delete(&model.ArticleDetail{}, "article_id = ?", item.Id)
		}

		if len(items) < size {
			break
		}

		lastId = items[size-1].Id
	}
}
