package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"time"

	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"
)

func loadConfigFile(configType, configLocation, consulKey string) {
	setDefaults()
	if configLocation == "" {
		return
	}

	switch configType {
	case "local":
		viper.SetConfigName("config")
		viper.AddConfigPath(configLocation)

		err := viper.ReadInConfig()
		if err != nil {
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}

		viper.WatchConfig()

	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})
}

func setDefaults() {
	viper.SetDefault("server.port", 3000)
	viper.SetDefault("backends.numConns", 3)
	viper.SetDefault("metrics.port", 8080)
}
