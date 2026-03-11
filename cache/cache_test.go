package cache

import (
	"fmt"
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

	v, exists = s.Get("This is a fake key")
	require.False(t, exists, "This key does not exist so should be false")
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

// TEST LRU POLICY - FORCE EVICTIONS
func TestLRUBasic(t *testing.T) {
	s := NewCache()
	s.policy = NewLRUPolicy()

	key1 := "1"
	value1 := &Data{
		Perm: false,
		Data: []byte("This is non-perm"),
	}
	key2 := "2"
	value2 := &Data{
		Perm: true,
		Data: []byte("This is perm"),
	}
	key3 := "3"
	value3 := &Data{
		Perm: false,
		Data: []byte("non-perm again"),
	}
	s.Set(key1, value1)
	s.Set(key2, value2)
	s.Set(key3, value3)

	v1, exists1 := s.Get(key1)
	require.True(t, exists1, "Key should exist in data")
	require.Equal(t, value1.Data, v1, "Retrieved value should be equal")

	v2, exists2 := s.Get(key2)
	require.True(t, exists2, "Key should exist in data")
	require.Equal(t, value2.Data, v2, "Retrieved value should be equal")

	v3, exists3 := s.Get(key3)
	require.True(t, exists3, "Key should exist in data")
	require.Equal(t, value3.Data, v3, "Retrieved value should be equal")

	//key 1 should be the last in line - lets force evict:
	k := s.policy.SelectVictim()
	s.Delete(k)
	_, exists1 = s.Get(key1)
	require.False(t, exists1, "This key should have been reaped")
	_, exists2 = s.Get(key2)
	require.True(t, exists2, "This key should never be reaped unless manually")
	_, exists3 = s.Get(key3)
	require.True(t, exists3, "Key should still be in the data")

	//Now 3 was most recently accessed but 2 is permmed, so if we reap again only 2 should be left
	k = s.policy.SelectVictim()
	s.Delete(k)
	fmt.Println(k)
	_, exists2 = s.Get(key2)
	require.True(t, exists2, "Permed data should not be reaped")
	_, exists3 = s.Get(key3)
	require.False(t, exists3, "Only other option to reap, so this should be reaped")
}

func TestLRUUpdate(t *testing.T) {
	s := NewCache()
	s.policy = NewLRUPolicy()

	key1 := "1"
	key2 := "2"
	value1 := &Data{
		Perm: false,
		Data: []byte("Hello World"),
	}
	value2 := &Data{
		Perm: false,
		Data: []byte("I'm here for a good time, not a long time"),
	}
	value3 := &Data{
		Perm: false,
		Data: []byte("A whole new world"),
	}

	s.Set(key1, value1)
	s.Set(key2, value2)
	v1, exists1 := s.Get(key1)
	require.True(t, exists1, "Value must be in cache")
	require.Equal(t, value1.Data, v1, "value must be what we put there")
	v2, exists2 := s.Get(key2)
	require.True(t, exists2, "Value 2 must be in data")
	require.Equal(t, value2.Data, v2, "value must be what we put there")

	s.Set(key1, value3)
	//Reap here after set before a get
	k := s.policy.SelectVictim()
	s.Delete(k)

	v1, exists1 = s.Get(key1)
	_, exists2 = s.Get(key2)
	require.True(t, exists1, "Key one should not be reaped")
	require.False(t, exists2, "Key two should be reaped")
	require.Equal(t, v1, value3.Data, "Key one should contain updated data")
}

func TestLRUPerm(t *testing.T) {
	s := NewCache()
	s.policy = NewLRUPolicy()

	//Calling get and delete on keys that don't exist, also selecting victim from an empty policy
	s.Get("1")
	s.Delete("1")
	key := s.policy.SelectVictim()
	require.Equal(t, key, "", "Victim should be the empty string")

	key1 := "1"
	value1 := &Data{
		Data: []byte("Hi mom, I am coding"),
	}
	key2 := "2"
	value2 := &Data{
		Data: []byte("Are you proud of me dad?"),
	}

	s.Set(key1, value1)
	s.Set(key2, value2)

	s.MakePerm(key1)
	k := s.policy.SelectVictim()
	s.Delete(k)
	v1, exists1 := s.Get(key1)
	_, exists2 := s.Get(key2)
	require.True(t, exists1, "This key was permmed so should not have been removed")
	require.Equal(t, v1, value1.Data, "Data is what we inserted")
	require.False(t, exists2, "While this was moved to the head, this is the only non-permmed data")
}

// TEST LRU POLICY - IMPLEMENT SIZE LIMITS
func TestLRUWithSize(t *testing.T) {
	size := 200

	//size of the cache item should be 88 + len(slice)
	//the flat overhead is for a bool, the padding, two time.Time structs, the int count, and 24 byte header for the Data slice
	//given this, we should be able to hold 2 10 byte objects, but should not be able to hold the 1000 byte data

	s := NewCache()
	s.policy = NewLRUPolicy()
	s.maxSize = size

	//Generate 10 byte data of a's
	tenByte := []byte{}
	for i := 1; i <= 10; i++ {
		tenByte = append(tenByte, 'a')
	}

	BIGBYTE := []byte{}
	for i := 1; i <= 1000; i++ {
		BIGBYTE = append(BIGBYTE, 'a')
	}

	key1 := "1"
	value1 := &Data{
		Data: tenByte,
	}
	value2 := &Data{
		Data: BIGBYTE,
	}
	key2 := "2"
	key3 := "3"
	key4 := "4"

	s.Set(key1, value1)
	s.Set(key2, value1)

	v1, exists1 := s.Get(key1)
	require.True(t, exists1, "Item 1 should not have been evicted when we set the value")
	require.Equal(t, v1, tenByte, "This should be the generic tenByte object")
	v2, exists2 := s.Get(key2)
	require.True(t, exists2, "the value 2 should have been set")
	require.Equal(t, v2, tenByte, "Should be tenByte")

	//now try some eviction:
	s.Set(key3, value1)
	_, exists3 := s.Get(key3)
	_, exists1 = s.Get(key1)
	require.True(t, exists3, "exists3 should be in the cache")
	require.False(t, exists1, "This should have been the last eviction")

	//Attempt to add an object that is too large to the cache
	s.Set(key4, value2)
	_, exists4 := s.Get(key4)
	require.False(t, exists4, "This item is too big to fit into the cache")

	//Make key 3 perm
	s.MakePerm(key3)
	s.Get(key2)
	s.Set(key1, value1) //this should cause an eviction of key2 even though it was more recently grabbed
	_, exists2 = s.Get(key2)
	require.False(t, exists2, "Should be evicted because the other option is permmed")
}

func TestLFUWithSize(t *testing.T) {
	s := NewCache()
	s.policy = NewLFUPolicy()

	tenByte := []byte{}
	for i := 1; i <= 10; i++ {
		tenByte = append(tenByte, 'a')
	}

	BIGBYTE := []byte{}
	for i := 1; i <= 1000; i++ {
		BIGBYTE = append(BIGBYTE, 'a')
	}

	size := 200
	s.SetSize(size)

	key1 := "1"
	key2 := "2"
	key3 := "3"
	key4 := "4"
	key5 := "5"
	key6 := "6"

	value1 := &Data{
		Data: tenByte,
	}
	value2 := &Data{
		Data: tenByte,
	}
	value3 := &Data{
		Data: tenByte,
	}
	value4 := &Data{
		Data: tenByte,
	}
	value5 := &Data{
		Data: tenByte,
	}
	value6 := &Data{
		Data: BIGBYTE,
	}

	s.Set(key1, value1)
	v1, exists1 := s.Get(key1)
	require.True(t, exists1, "Value should be in cache")
	require.Equal(t, v1, value1.Data, "Data should be equal to inserted data")

	s.Set(key2, value2)
	s.Set(key3, value3)
	_, exists2 := s.Get(key2)
	v3, exists3 := s.Get(key3)
	require.False(t, exists2, "key2 should have been reaped from bucket 1")
	require.True(t, exists3, "Key 3 should now be present in bucket 2")
	require.Equal(t, v3, value3.Data, "Key 3's data should be correct")
	require.Equal(t, value3.Count, 2, "Should be in bucket2 - one for creation one for get")

	s.AddSize(100)
	s.Set(key4, value4)
	s.Get(key4)
	v4, exists4 := s.Get(key4)
	require.True(t, exists4, "Key 4 should exist and be present in bucket 3")
	require.Equal(t, v4, value4.Data)
	require.Equal(t, value4.Count, 3, "key 4 should be in bucket 3")

	//Now if we eject a value, it should be key1 from bucket 2
	s.Set(key5, value5)
	_, exists1 = s.Get(key1)
	require.False(t, exists1, "This value should have been ejected from bucket 2")

	s.Set(key6, value6)
	_, exists6 := s.Get(key6)
	require.False(t, exists6, "This value is to big to include in the cache")
}
