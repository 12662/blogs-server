package service

import (
	"server/config"
	"server/global"
	"server/model/appTypes"
	"server/utils"
	"strings"

	"gorm.io/gorm"
)

type ConfigService struct {
}

func (configService *ConfigService) UpdateWebsite(website config.Website) error {
	oldArray := []string{
		global.Config.Website.Logo,
		global.Config.Website.FullLogo,
		global.Config.Website.QQImage,
		global.Config.Website.WechatImage,
	}

	newArray := []string{
		website.Logo,
		website.FullLogo,
		website.QQImage,
		website.WechatImage,
	}

	added, removed := utils.DiffArrays(oldArray, newArray)

	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := utils.InitImagesCategory(global.DB, removed); err != nil {
			return err
		}
		if err := utils.ChangeImagesCategory(global.DB, added, appTypes.System); err != nil {
			return err
		}
		global.Config.Website = website
		if err := utils.SaveYAML(); err != nil {
			return err
		}
		return nil
	})
}

func (configService *ConfigService) UpdateSystem(system config.System) error {
	global.Config.System.UseMultipoint = system.UseMultipoint
	global.Config.System.SessionsSecret = system.SessionsSecret
	global.Config.System.OssType = system.OssType
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateEmail(email config.Email) error {
	global.Config.Email = email
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateQQ(qq config.QQ) error {
	global.Config.QQ = qq
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateQiniu(qiniu config.Qiniu) error {
	global.Config.Qiniu = qiniu
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateJwt(jwt config.Jwt) error {
	global.Config.Jwt = jwt
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateGaode(gaode config.Gaode) error {
	global.Config.Gaode = gaode
	return utils.SaveYAML()
}

func (configService *ConfigService) UpdateAI(ai config.AI) error {
	ai.QwenEmbeddingModel = global.Config.AI.QwenEmbeddingModel
	ai.EmbeddingDimensions = global.Config.AI.EmbeddingDimensions
	ai.ChatModels = normalizeAIModels(ai.ChatModels)
	global.Config.AI = ai
	return utils.SaveYAML()
}

func normalizeAIModels(models []config.AIModel) []config.AIModel {
	result := make([]config.AIModel, 0, len(models))
	seen := make(map[string]struct{})
	for _, model := range models {
		model.Name = strings.TrimSpace(model.Name)
		model.ExpireAt = strings.TrimSpace(model.ExpireAt)
		if model.Name == "" {
			continue
		}
		key := model.Name + "\x00" + model.ExpireAt
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	return result
}
