package task

import (
	"server/global"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func RegisterScheduledTasks(c *cron.Cron) error {
	if _, err := c.AddFunc("@hourly", func() {
		if err := UpdateArticleViewsSyncTask(); err != nil {
			global.Log.Error("Failed to update article views:", zap.Error(err))
		}
	}); err != nil {
		return err
	}
	if _, err := c.AddFunc("@hourly", func() {
		if err := GetHotListSyncTask(); err != nil {
			global.Log.Error("Failed to get hot list:", zap.Error(err))
		}
	}); err != nil {
		return err
	}
	if _, err := c.AddFunc("@daily", func() {
		if err := GetCalendarSyncTask(); err != nil {
			global.Log.Error("Failed to get calendar:", zap.Error(err))
		}
	}); err != nil {
		return err
	}
	if global.Config.AI.RAGMaintenanceEnable {
		spec := global.Config.AI.RAGMaintenanceSpec
		if spec == "" {
			spec = "@every 30m"
		}
		if _, err := c.AddFunc(spec, func() {
			result, err := MaintainRAGSyncTask()
			if err != nil {
				global.Log.Error("Failed to maintain rag sync:", zap.Error(err))
				return
			}
			global.Log.Info("RAG maintenance finished",
				zap.Int("scanned", result.Scanned),
				zap.Int("queued", result.Queued),
				zap.Int("skipped", result.Skipped),
				zap.Int("failed", result.Failed),
			)
		}); err != nil {
			return err
		}
	}
	return nil
}
