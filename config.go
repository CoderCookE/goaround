package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"

	"github.com/spf13/viper"
)

func loadConfigFile(configLocation string) {
	setDefaults()
	if configLocation != "" {
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
