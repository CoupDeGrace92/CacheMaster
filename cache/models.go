package cache

import (
	"math"
	"sync"
	"time"
	"unsafe"
)

type data struct {
	Perm       bool
	CreatedAt  time.Time
	LastAccess time.Time
	Count      int
	Data       []byte
}

type cache struct {
	mu              sync.Mutex
	data            map[string]*data
	perm            map[string]*data
	policy          EvictionPolicy
	maxSize         int
	currentSize     int
	currentPermSize int
	Reapers         []*managedReaper
}

type managedReaper struct {
	Loop   TimeReap
	StopCh chan struct{}
}

func (m *managedReaper) Close() {
	close(m.StopCh)
}

func (m *managedReaper) Start(interval time.Duration, c *cache) {
	s := m.Loop.Reap(interval, c)
	m.StopCh = s
}

func (m *managedReaper) onInsert(key string, entry *data) {
	m.Loop.onInsert(key, entry)
}

func (m *managedReaper) onAccess(key string, entry *data) {
	m.Loop.onAccess(key, entry)
}

func (m *managedReaper) onDelete(key string, entry *data) {
	m.Loop.onDelete(key, entry)
}

func (c *cache) AddManagedReaper(m TimeReap) {
	r := &managedReaper{
		Loop: m,
	}
	c.Reapers = append(c.Reapers, r)
}

func NewCache() *cache {
	d := make(map[string]*data)
	p := make(map[string]*data)
	c := cache{
		mu:      sync.Mutex{},
		data:    d,
		perm:    p,
		maxSize: math.MaxInt,
	}
	return &c
}

func (c *cache) Get(key string) ([]byte, bool) {
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
		c.policy.onAccess(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onAccess(key, d)
	}
	return d.Data, exists
}

func (c *cache) SetSize(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize = i
	if c.maxSize < 0 {
		c.maxSize = 0
	}
}

func (c *cache) AddSize(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize += i
	if c.maxSize < 0 {
		c.maxSize = 0
	}
}

func (c *cache) GetMaxSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxSize
}

func (c *cache) GetCurrentSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentSize
}

func (c *cache) GetCurrentPermSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentPermSize
}

func (c *cache) Set(key string, d *data) (added bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	added = true
	//If key already exists and we are just updating, our sizing function has to account for that
	current := 0
	if dat, ok := c.data[key]; ok {
		current = dat.sizeOf()
	} else if dat, ok := c.perm[key]; ok {
		current = dat.sizeOf()
	}
	//Below we have exactly what we would want if it is not currently in the map
	if c.policy != nil {
		add := c.Sizing(d, current)
		if !add {
			return false
		}
	} else {
		if d.sizeOf() > c.maxSize-c.currentSize {
			return false
		}
		c.currentSize += d.sizeOf() - current
		if d.Perm {
			c.currentPermSize += d.sizeOf() - current
		}
	}
	if d.Perm == true {
		c.perm[key] = d
	} else {
		c.data[key] = d
	}
	if c.policy != nil && d.Perm == false {
		c.policy.onInsert(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onInsert(key, d)
	}
	return
}

func (c *cache) MakePerm(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[key]
	if !ok {
		return
	}
	c.perm[key] = d
	delete(c.data, key)
	if c.policy != nil {
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onDelete(key, d)
	}
	c.currentPermSize += d.sizeOf()
}

func (c *cache) MakeNonPerm(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.perm[key]
	if !ok {
		return
	}
	c.data[key] = d
	delete(c.perm, key)
	if c.policy != nil {
		c.policy.onInsert(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onDelete(key, d)
	}
	c.currentPermSize -= d.sizeOf()
}

func (c *cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[key]
	if !ok {
		d, ok = c.perm[key]
		if !ok {
			return
		}
	}
	c.currentSize -= d.sizeOf()
	delete(c.data, key)
	delete(c.perm, key)
	if c.policy != nil && !d.Perm {
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onDelete(key, d)
	}
}

// We need a function when the mutex is locked externally
func (c *cache) deleteNoLock(key string) {
	d, ok := c.data[key]
	if !ok {
		d, ok = c.perm[key]
		if !ok {
			return
		}
	}
	c.currentSize -= d.sizeOf()
	delete(c.data, key)
	delete(c.perm, key)
	if c.policy != nil && !d.Perm {
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.Reapers {
		reaper.onDelete(key, d)
	}
}

func (d *data) sizeOf() int {
	fixedSize := unsafe.Sizeof(data{})
	variableSize := len(d.Data)
	return int(fixedSize) + variableSize
}

// This function is responsible for implementing the policy
func (c *cache) Sizing(d *data, currentDataSize int) (add bool) {
	add = true
	size := d.sizeOf() - currentDataSize
	if size < 0 {
		c.currentSize += size
		return
	}
	if size >= c.maxSize-c.currentPermSize {
		add = false
		return
	}
	for size+c.currentSize >= c.maxSize {
		key := c.policy.selectVictim()
		if data, ok := c.data[key]; ok {
			c.policy.onDelete(key, data)
			c.deleteNoLock(key)
		}
	}
	c.currentSize += size
	if d.Perm {
		c.currentPermSize += size
	}
	return
}
