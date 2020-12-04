package main

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestParseFlags(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	t.Run("Returns defaults", func(t *testing.T) {
		parseConfig()

		assertion.Equal(":3000", fmt.Sprintf(":%d", viper.GetInt("server.port")))
		assertion.Equal(":8080", fmt.Sprintf(":%d", viper.GetInt("metrics.port")))
		assertion.Equal(fmt.Sprintf("%v", make([]string, 0)), fmt.Sprintf("%v", viper.GetStringSlice("backends.hosts")))
		assertion.Equal(3, viper.GetInt("backends.numConns"))
		assertion.Equal("", viper.GetString("server.cacert"))
		assertion.Equal("", viper.GetString("server.privkey"))
		assertion.Equal(false, viper.GetBool("cache.enabled"))
	})
}
