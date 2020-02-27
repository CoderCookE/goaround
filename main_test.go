package main

import (
	"github.com/CoderCookE/goaround/internal/assert"
	"testing"
)

func TestParseFlags(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	t.Run("Returns defaults", func(t *testing.T) {
		portString, metricPortString, backends, numConns, cacert, privkey, cache := parseFlags()
		assertion.Equal(":3000", portString)
		assertion.Equal(":8080", metricPortString)
		assertion.Equal(backends.String(), "[]")
		assertion.Equal(*numConns, 3)
		assertion.Equal(*cacert, "")
		assertion.Equal(*privkey, "")
		assertion.Equal(*cache, false)
	})
}
