package cache

import (
	"math"
	"sync"
	"time"
	"unsafe"
)

type Data struct {
	Perm       bool
	CreatedAt  time.Time
	LastAccess time.Time
	Count      int
	Data       []byte
}

type Cache struct {
	mu          sync.Mutex
	data        map[string]*Data
	perm        map[string]*Data
	policy      EvictionPolicy
	maxSize     int
	currentSize int
}

func NewCache() *Cache {
	d := make(map[string]*Data)
	p := make(map[string]*Data)
	c := Cache{
		mu:      sync.Mutex{},
		data:    d,
		perm:    p,
		maxSize: math.MaxInt,
	}
	return &c
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, exists := c.data[key]
	if !exists {
		d, exists = c.perm[key]
		if !exists {
			return nil, exists
		}
	}
	if c.policy != nil && d.Perm == false {
		c.policy.OnAccess(key, d)
	}
	return d.Data, exists
}

func (c *Cache) Set(key string, d *Data) {
	c.mu.Lock()
	defer c.mu.Unlock()
	//If key already exists and we are just updating, our sizing function has to account for that
	current := 0
	if dat, ok := c.data[key]; ok {
		current = dat.SizeOf()
	} else if dat, ok := c.perm[key]; ok {
		current = dat.SizeOf()
	}
	//Below we have exactly what we would want if it is not currently in the map
	if c.policy != nil {
		add := c.Sizing(d, current)
		if !add {
			return
		}
	} else {
		if d.SizeOf() > c.maxSize-c.currentSize {
			return
		}
		c.currentSize += d.SizeOf()
	}
	if d.Perm == true {
		c.perm[key] = d
	} else {
		c.data[key] = d
	}
	if c.policy != nil && d.Perm == false {
		c.policy.OnInsert(key, d)
	}
}

func (c *Cache) MakePerm(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[key]
	if !ok {
		return
	}
	c.perm[key] = d
	delete(c.data, key)
	if c.policy != nil {
		c.policy.OnDelete(key, d)
	}
}

func (c *Cache) MakeNonPerm(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.perm[key]
	if !ok {
		return
	}
	c.data[key] = d
	delete(c.perm, key)
	if c.policy != nil {
		c.policy.OnInsert(key, d)
	}
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[key]
	if !ok {
		d, ok = c.perm[key]
		if !ok {
			return
		}
	}
	c.currentSize -= d.SizeOf()
	delete(c.data, key)
	delete(c.perm, key)
	if c.policy != nil && !d.Perm {
		c.policy.OnDelete(key, d)
	}
}

func (d *Data) SizeOf() int {
	fixedSize := unsafe.Sizeof(Data{})
	variableSize := len(d.Data)
	return int(fixedSize) + variableSize
}

// This function is responsible for implementing the policy
func (c *Cache) Sizing(d *Data, currentDataSize int) (add bool) {
	add = true
	size := d.SizeOf() - currentDataSize
	if size < 0 {
		c.currentSize += size
		return
	}
	if size >= c.maxSize {
		add = false
		return
	}
	for size+c.currentSize >= c.maxSize {
		key := c.policy.SelectVictim()
		c.currentSize -= c.data[key].SizeOf()
		delete(c.data, key)
	}
	c.currentSize += size
	return
}
