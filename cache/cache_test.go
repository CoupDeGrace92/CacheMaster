package cache

import (
	"testing"

	"github.com/strtchr/testify/require"
)

func TestSimpleStorage(t *testing.T) {
	s := NewCache()
	key := "testing"
	value := &Data{
		Perm: false,
		Data: []byte("This is a test"),
	}

	s.Set(key, value)
	v, exists := s.Get(key)

	require.True(t, exists, "Key should exist in storage")
	require.Equal(t, value.Data, v, "Retrieved value should be the same as the outputted value")
}
