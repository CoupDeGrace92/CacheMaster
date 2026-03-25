package cache

import (
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"
)

type Data struct {
	perm       bool
	CreatedAt  time.Time
	LastAccess time.Time
	TTL        time.Duration
	Count      int
	data       []byte
}

type Cache struct {
	mu              sync.Mutex
	data            map[string]*Data
	perm            map[string]*Data
	policy          EvictionPolicy
	maxSize         int
	currentSize     int
	currentPermSize int
	reapers         []*managedReaper
}

type managedReaper struct {
	Loop   TimeReap
	StopCh chan struct{}
}

func NewManagedReaper(t TimeReap) *managedReaper {
	m := managedReaper{
		Loop:   t,
		StopCh: make(chan struct{}),
	}
	return &m
}

func (m *managedReaper) Close() {
	close(m.StopCh)
}

func (m *managedReaper) Start(interval time.Duration, c *Cache) {
	s := m.Loop.Reap(interval, c)
	m.StopCh = s
}

func (m *managedReaper) onInsert(key string, entry *Data) {
	m.Loop.onInsert(key, entry)
}

func (m *managedReaper) onAccess(key string, entry *Data) {
	m.Loop.onAccess(key, entry)
}

func (m *managedReaper) onDelete(key string, entry *Data) {
	m.Loop.onDelete(key, entry)
}

func (c *Cache) AddManagedReaper(m TimeReap) {
	r := &managedReaper{
		Loop: m,
	}
	c.reapers = append(c.reapers, r)
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

func (c *Cache) SetEvictionPolicy(m EvictionPolicy) error {
	if c.policy != nil {
		return fmt.Errorf("Eviction policy already exists")
	}
	c.policy = m
	return nil
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
	if c.policy != nil && d.perm == false {
		c.policy.onAccess(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onAccess(key, d)
	}
	return d.data, exists
}

func (c *Cache) SetSize(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize = i
	if c.maxSize < 0 {
		c.maxSize = 0
	}
}

func (c *Cache) AddSize(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize += i
	if c.maxSize < 0 {
		c.maxSize = 0
	}
}

func (c *Cache) GetMaxSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxSize
}

func (c *Cache) GetCurrentSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentSize
}

func (c *Cache) GetCurrentPermSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentPermSize
}

func newData(v []byte) *Data {
	d := Data{
		data: v,
	}
	return &d
}

func (c *Cache) Set(key string, v []byte) (added bool) {
	d := newData(v)
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
		add := c.sizing(d, current)
		if !add {
			return false
		}
	} else {
		if d.sizeOf() > c.maxSize-c.currentSize {
			return false
		}
		c.currentSize += d.sizeOf() - current
		if d.perm {
			c.currentPermSize += d.sizeOf() - current
		}
	}
	if d.perm == true {
		c.perm[key] = d
	} else {
		c.data[key] = d
	}
	if c.policy != nil && d.perm == false {
		c.policy.onInsert(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onInsert(key, d)
	}
	return
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
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onDelete(key, d)
	}
	c.currentPermSize += d.sizeOf()
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
		c.policy.onInsert(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onDelete(key, d)
	}
	c.currentPermSize -= d.sizeOf()
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
	c.currentSize -= d.sizeOf()
	delete(c.data, key)
	delete(c.perm, key)
	if c.policy != nil && !d.perm {
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onDelete(key, d)
	}
}

// We need a function when the mutex is locked externally
func (c *Cache) deleteNoLock(key string) {
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
	if c.policy != nil && !d.perm {
		c.policy.onDelete(key, d)
	}
	for _, reaper := range c.reapers {
		reaper.onDelete(key, d)
	}
}

func (d *Data) sizeOf() int {
	fixedSize := unsafe.Sizeof(Data{})
	variableSize := len(d.data)
	return int(fixedSize) + variableSize
}

// This function is responsible for implementing the policy
func (c *Cache) sizing(d *Data, currentDataSize int) (add bool) {
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
	if d.perm {
		c.currentPermSize += size
	}
	return
}
