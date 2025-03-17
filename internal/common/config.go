package common

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

func LoadConfig(configName string) {
	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(os.Getenv("CONFIG_PATH"))

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Sprintf("[%s] viper.ReadInConfig, err %v", "common.config", err))
	}
}
