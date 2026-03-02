package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TESTS FOR POLICY-LESS STORAGE
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

	s.Delete(key)
	v, exists = s.Get(key)
	require.False(t, exists, "Key should be deleted from the storage")
}

func TestPermNonPerm(t *testing.T) {
	s := NewCache()
	key := "perm"
	value := &Data{
		Perm: true,
		Data: []byte("This is permanent"),
	}

	s.Set(key, value)
	v, exists := s.Get(key)

	require.True(t, exists, "Key should exist even in perm storage")
	require.Equal(t, value.Data, v, "Retrieved value should be the same as the outputted value")

	s.MakeNonPerm("perm")

	v, exists = s.Get(key)

	require.True(t, exists, "Key should exist even in non-perm storage")
	require.Equal(t, value.Data, v, "Retrieved value should be the same as the outputted value")

	s.MakePerm("perm")

	v, exists = s.Get(key)

	require.True(t, exists, "Key should exist even in perm storage, after moving")
	require.Equal(t, value.Data, v, "Retrieved value should be the same as the outputted value")

	s.MakePerm("perm")

	v, exists = s.Get(key)
	require.True(t, exists, "Key should exist even after permming 2 times in a row")
	require.Equal(t, value.Data, v, "Retrieved value should be the same as the outputted value")
}

func TestDeleteBasic(t *testing.T) {
	s := NewCache()
	key1 := "non-perm"
	value1 := &Data{
		Perm: false,
		Data: []byte("This is not permanent"),
	}
	key2 := "perm"
	value2 := &Data{
		Perm: true,
		Data: []byte("This is permanent"),
	}

	s.Set(key1, value1)
	s.Set(key2, value2)

	v1, exists1 := s.Get(key1)
	require.True(t, exists1, "Key should exist for non-perm")
	require.Equal(t, value1.Data, v1, "Retrieved value should be the same as the outputted value")

	v2, exists2 := s.Get(key2)
	require.True(t, exists2, "Key should exist for perm data")
	require.Equal(t, value2.Data, v2, "Retrieved value should be equal")

	s.Delete(key1)
	s.Delete(key2)

	v1, exists1 = s.Get(key1)
	_, exists2 = s.Get(key2)
	require.False(t, exists1, "Key should have been removed for non-perm")
	require.False(t, exists2, "Key should be deleted for permmed data")
	//following should not panic:
	s.Delete("Not a real key")
}

//TEST LRU POLICY - FORCE EVICTIONS

//TEST LRU POLICY - IMPLEMENT SIZE LIMITS
