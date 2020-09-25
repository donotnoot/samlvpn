package main

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepeatingBuffer(t *testing.T) {
	s := "string"
	r := NewRepeatingBuffer(s)

	b, err := ioutil.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, s, string(b))

	b, err = ioutil.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, s, string(b))

	b, err = ioutil.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, s, string(b))
}
