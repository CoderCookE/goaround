package main

import (
	"github.com/codercooke/goaround/internal/assert"
	"testing"
)

func TestParseFlags(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	t.Run("Returns defaults", func(t *testing.T) {
		portString, backends := parseFlags()
		assertion.Equal(":3000", portString)
		assertion.Equal(backends.String(), "[]")
	})
}
