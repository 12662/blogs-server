package utils

import (
	"io/fs"
	"os"
	"server/global"

	"gopkg.in/yaml.v3"
)

const ConfigFile = "config.yaml"

func LoadYAML() ([]byte, error) {
	return os.ReadFile(ConfigFile)
}

func SaveYAML() error {
	byteData, err := yaml.Marshal(global.Config)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, byteData, fs.ModePerm)
}
