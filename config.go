package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"time"

	"github.com/spf13/viper"
)

func loadConfigFile(configType, configLocation, consulKey string) {
	setDefaults()
	if configLocation == "" {
		return
	}

	switch configType {
	case "consul":
		viper.AddRemoteProvider(configType, configLocation, consulKey)
		viper.SetConfigType("json")
		err := viper.ReadRemoteConfig()
		if err != nil {
			fmt.Errorf("Fatal error config file: %s \n, retrying", err)
			time.Sleep(500)
			loadConfigFile(configType, configLocation, consulKey)
		}

		viper.WatchRemoteConfig()
		viper.OnConfigChange(func(e fsnotify.Event) {
			fmt.Println("Config file changed:", e.Name)
		})

	case "local":
		viper.SetConfigName("config")
		viper.AddConfigPath(configLocation)

		err := viper.ReadInConfig()
		if err != nil {
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}

		viper.WatchConfig()
		viper.OnConfigChange(func(e fsnotify.Event) {
			fmt.Println("Config file changed:", e.Name)
		})
	}
}

func setDefaults() {
	viper.SetDefault("server.port", 3000)
	viper.SetDefault("backends.numConns", 3)
	viper.SetDefault("metrics.port", 8080)
}
