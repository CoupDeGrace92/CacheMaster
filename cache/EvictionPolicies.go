package cache

import (
	"fmt"
	"math"
	"time"
)

type TimeReap interface {
	onInsert(key string, entry *Data)
	onAccess(key string, entry *Data)
	onDelete(key string, entry *Data)
	Reap(interval time.Duration, cache *Cache) chan struct{}
}

type EvictionPolicy interface {
	OnInsert(key string, entry *Data)
	OnAccess(key string, entry *Data)
	OnDelete(key string, entry *Data)
	Contains(key string) bool
	SelectVictim() string
	//think about adding OnUpdate
}

// THE FIRST POLICY - LRU
type LRUPolicy struct {
	head    *LLnode
	tail    *LLnode
	nodeMap map[string]*LLnode
}

type LLnode struct {
	key  string
	prev *LLnode
	next *LLnode
}

func NewLRUPolicy() *LRUPolicy {
	return &LRUPolicy{
		nodeMap: make(map[string]*LLnode),
	}
}

func (p *LRUPolicy) Contains(key string) bool {
	_, ok := p.nodeMap[key]
	return ok
}

func (p *LRUPolicy) OnInsert(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		node = &LLnode{
			key: key,
		}
		p.nodeMap[key] = node
	}
	entry.Count++
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.LastAccess = time.Now()
	p.moveToHead(node)
}

func (p *LRUPolicy) OnAccess(key string, entry *Data) {
	node := p.nodeMap[key]
	entry.Count++
	entry.LastAccess = time.Now()
	p.moveToHead(node)
}

func (p *LRUPolicy) OnDelete(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		return
	}
	p.removeNode(node)
	delete(p.nodeMap, key)
}

func (p *LRUPolicy) SelectVictim() (key string) {
	if p.tail == nil {
		return ""
	}
	key = p.tail.key
	return
}

func (p *LRUPolicy) moveToHead(c *LLnode) {
	if p.head == c {
		return
	}
	if c.prev != nil {
		c.prev.next = c.next
	}
	if c.next != nil {
		c.next.prev = c.prev
	}
	if p.tail == c {
		p.tail = c.prev
	}

	c.prev = nil
	c.next = p.head
	if p.head != nil {
		p.head.prev = c
	}
	p.head = c
	if p.tail == nil {
		p.tail = c
	}
}

func (p *LRUPolicy) removeNode(c *LLnode) {
	if p.head == p.tail {
		p.head = nil
		p.tail = nil
		return
	}
	if c.prev != nil {
		c.prev.next = c.next
	} else {
		p.head = c.next
	}

	if c.next != nil {
		c.next.prev = c.prev
	} else {
		p.tail = c.prev
	}
}

// THE SECOND POLICY - LFU
type LFUPolicy struct {
	buckets map[int]*Bucket
	nodeMap map[string]*LLnode
	minFreq int
}

type Bucket struct {
	head *LLnode
	tail *LLnode
	next *Bucket
	prev *Bucket
	id   int
}

func NewLFUPolicy() *LFUPolicy {
	return &LFUPolicy{
		minFreq: 0,
		buckets: make(map[int]*Bucket),
		nodeMap: make(map[string]*LLnode),
	}
}

func (p *LFUPolicy) Contains(key string) bool {
	_, ok := p.nodeMap[key]
	return ok
}

func (p *LFUPolicy) addToBucket(node *LLnode, prevBucket *Bucket, entry *Data) {
	bucket, ok := p.buckets[entry.Count]
	if !ok {
		var next *Bucket
		if prevBucket != nil {
			next = prevBucket.next
		} else {
			if p.minFreq != 0 && p.minFreq != math.MaxInt {
				next = p.buckets[p.minFreq]
			}
		}
		p.buckets[entry.Count] = &Bucket{
			head: node,
			tail: node,
			next: next,
			prev: prevBucket,
			id:   entry.Count,
		}
		bucket = p.buckets[entry.Count]
		if bucket.next != nil {
			bucket.next.prev = bucket
		}
		if bucket.prev != nil {
			bucket.prev.next = bucket
		}

		if entry.Count < p.minFreq || p.minFreq == 0 {
			p.minFreq = entry.Count
		}
	} else {
		h := bucket.head
		node.next = h
		h.prev = node
		bucket.head = node
	}
}

func (p *LFUPolicy) removeFromBucketTail() (key string) {
	if len(p.buckets) == 0 {
		return
	}
	bucket := p.buckets[p.minFreq]
	if bucket.head == bucket.tail {
		key = bucket.head.key
		if bucket.next != nil {
			bucket.next.prev = nil
		}
		delete(p.buckets, p.minFreq)
		newMin := math.MaxInt
		for i, _ := range p.buckets {
			if i < newMin {
				newMin = i
			}
		}
		p.minFreq = newMin
		return
	}
	node := bucket.tail
	newTail := bucket.tail.prev
	newTail.next = nil
	bucket.tail = newTail
	node.prev = nil
	key = node.key
	return
}

func (p *LFUPolicy) removeFromBucket(entry *Data, node *LLnode) (previous *Bucket) {
	bucket := p.buckets[entry.Count]
	if bucket.head == bucket.tail {
		n := bucket.next
		prev := bucket.prev
		if n != nil {
			n.prev = prev
		}
		if prev != nil {
			prev.next = n
		}
		delete(p.buckets, entry.Count)
		bucket.next = nil
		bucket.prev = nil
		if p.minFreq == entry.Count {
			newMin := math.MaxInt
			for i, _ := range p.buckets {
				if i < newMin {
					newMin = i
				}
			}
			p.minFreq = newMin
		}
		previous = prev
		return
	}
	n := node.next
	prev := node.prev
	if n != nil {
		n.prev = prev
	} else {
		bucket.tail = prev
	}
	if prev != nil {
		prev.next = n
	} else {
		bucket.head = n
	}
	previous = bucket
	//necessary to allow these objects to be garbage collected
	node.next = nil
	node.prev = nil
	return
}

func (p *LFUPolicy) promoteRemove(entry *Data, node *LLnode) (previous *Bucket) {
	bucket := p.buckets[entry.Count]
	if bucket.head == bucket.tail {
		previous = bucket.prev
		bn := p.buckets[entry.Count].next
		bp := p.buckets[entry.Count].prev
		if bn != nil {
			bn.prev = bp
		}
		if bp != nil {
			bp.next = bn
		}
		delete(p.buckets, entry.Count)
		if p.minFreq == entry.Count {
			p.minFreq++
		}
		return
	}
	n := node.next
	prev := node.prev
	if n != nil {
		n.prev = prev
	} else {
		bucket.tail = prev
	}
	if prev != nil {
		prev.next = n
	} else {
		bucket.head = n
	}
	node.next = nil
	node.prev = nil
	previous = bucket
	return
}

// Right now this function only works for items without history we create a data item with no
// history in the cache and insert it in the first bucket
func (p *LFUPolicy) OnInsert(key string, entry *Data) {
	entry.LastAccess = time.Now()
	if entry.Count > 1 {
		p.OnInsertGeneric(key, entry)
		return
	}
	entry.CreatedAt = time.Now()
	entry.Count = 1
	node := &LLnode{
		key: key,
	}
	p.nodeMap[key] = node
	p.addToBucket(node, nil, entry)
}

func (p *LFUPolicy) OnInsertGeneric(key string, entry *Data) {
	entry.LastAccess = time.Now()
	entry.Count++

	//We can speed this up by calling the generic method if the count is just one.
	if entry.Count == 1 {
		p.OnInsert(key, entry)
		return
	}
	node := &LLnode{
		key: key,
	}
	p.nodeMap[key] = node
	currentBucket, ok := p.buckets[p.minFreq]
	if !ok {
		p.addToBucket(node, nil, entry)
		return
	}
	for {
		if entry.Count == currentBucket.id {
			break
		}
		if entry.Count < currentBucket.id {
			if currentBucket.prev == nil {
				currentBucket = nil
			} else {
				currentBucket = currentBucket.prev
			}
			break
		}
		if currentBucket.next == nil {
			break
		}
		currentBucket = currentBucket.next
	}
	p.addToBucket(node, currentBucket, entry)
}

func (p *LFUPolicy) OnAccess(key string, entry *Data) {
	node := p.nodeMap[key]
	entry.LastAccess = time.Now()
	prev := p.promoteRemove(entry, node)
	entry.Count++
	p.addToBucket(node, prev, entry)
}

func (p *LFUPolicy) OnDelete(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		return
	}
	p.removeFromBucket(entry, node)
	delete(p.nodeMap, key)
}

func (p *LFUPolicy) SelectVictim() (key string) {
	key = p.removeFromBucketTail()
	delete(p.nodeMap, key)
	return
}

// TIME BASED REAP POLICIES
// Last Access reap
type LAReap struct {
	head   *LLnode
	tail   *LLnode
	m      map[string]*LLnode
	maxAge time.Duration
}

func NewLAReap(maxAge time.Duration) *LAReap {
	return &LAReap{
		m:      make(map[string]*LLnode),
		maxAge: maxAge,
	}
}

func (l *LAReap) onAccess(key string, entry *Data) {
	entry.LastAccess = time.Now()
	node := l.m[key]
	if l.head == node {
		return
	}
	if node.prev != nil {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}
	head := l.head
	l.head = node
	head.prev = node
	node.next = head
	node.prev = nil
}

func (l *LAReap) onInsert(key string, entry *Data) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.LastAccess = time.Now()

	node := &LLnode{
		key: key,
	}
	l.m[key] = node
	if l.head == nil {
		l.head = node
		l.tail = node
		return
	}

	l.head.prev = node
	node.next = l.head
	l.head = node
}

func (l *LAReap) onDelete(key string, entry *Data) {
	node, ok := l.m[key]
	if !ok {
		return
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}
	delete(l.m, key)
}

func (l *LAReap) Reap(interval time.Duration, cache *Cache) chan struct{} {
	stopChan := make(chan struct{})

	go func() {

		ticker := time.NewTicker(interval)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				l.Check(l.maxAge, cache)
			}
		}
	}()

	return stopChan
}

func (l *LAReap) Check(maxAge time.Duration, cache *Cache) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	for l.tail != nil {
		key := l.tail.key
		entry, ok := cache.data[key]
		if !ok {
			l.onDelete(key, nil)
			continue
		}

		if time.Since(entry.LastAccess) > maxAge {
			cache.DeleteNoLock(key)
		} else {
			break
		}
	}
}

// Time Since Created reap
type CAReap struct {
	head   *LLnode
	tail   *LLnode
	m      map[string]*LLnode
	maxAge time.Duration
}

func NewCAReap(maxAge time.Duration) *CAReap {
	return &CAReap{
		m:      make(map[string]*LLnode),
		maxAge: maxAge,
	}
}

func (c *CAReap) onAccess(key string, entry *Data) {
	entry.LastAccess = time.Now()
}

func (c *CAReap) onInsert(key string, entry *Data) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.LastAccess = time.Now()
	node := &LLnode{
		key: key,
	}
	c.m[key] = node

	if c.head == nil {
		c.head = node
		c.tail = node
		return
	}
	c.head.prev = node
	node.next = c.head
	c.head = node
}

func (c *CAReap) onDelete(key string, entry *Data) {
	node, ok := c.m[key]
	if !ok {
		return
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	delete(c.m, key)
}

func (c *CAReap) Reap(interval time.Duration, cache *Cache) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				c.Check(c.maxAge, cache)
			}
		}
	}()

	return stopChan
}

func (c *CAReap) Check(maxAge time.Duration, cache *Cache) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	for c.tail != nil {
		key := c.tail.key
		entry, ok := cache.data[key]
		if !ok {
			c.onDelete(key, nil)
			continue
		}

		if time.Since(entry.CreatedAt) > maxAge {
			cache.DeleteNoLock(key)
		} else {
			break
		}
	}
}

// Incubating New Cache ELements
type TieredPolicy struct {
	Nursery       EvictionPolicy
	NurseryReaper TimeReap
	Mature        EvictionPolicy
	sTime         time.Duration
	pFreq         int
	matureSize    int
	maxMatureSize int
	parentCache   *Cache
}

func NewTieredPolicy(Nursery, Mature EvictionPolicy, Reaper TimeReap, reapInterval time.Duration, c *Cache) (*TieredPolicy, error) {
	if Nursery == nil || Mature == nil || c == nil {
		return nil, fmt.Errorf("Nursery, Mature, and parent cache can not be nil")
	}

	if Reaper != nil && reapInterval == time.Duration(0) {
		return nil, fmt.Errorf("Reaper must have a reap interval")
	}
	t := &TieredPolicy{
		Nursery:       Nursery,
		NurseryReaper: Reaper,
		Mature:        Mature,
		sTime:         1000000 * time.Hour,
		pFreq:         math.MaxInt,
		maxMatureSize: math.MaxInt,
		parentCache:   c,
	}
	if Reaper != nil {
		t.NurseryReaper.Reap(reapInterval, c)
	}
	return t, nil
}

func (t *TieredPolicy) SetPromotionFreq(i int) {
	t.pFreq = i
}

func (t *TieredPolicy) SetSurvivalTime(d time.Duration) {
	t.sTime = d
}

func (t *TieredPolicy) SetMaxMatureSize(s int) {
	t.maxMatureSize = s
}

func (t *TieredPolicy) OnAccess(key string, entry *Data) {
	switch {
	case t.Nursery.Contains(key):
		t.Nursery.OnAccess(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onAccess(key, entry)
		}
	case t.Mature.Contains(key):
		t.Mature.OnAccess(key, entry)
	default:
	}
	if entry.Count >= t.pFreq || time.Now().After(entry.CreatedAt.Add(t.sTime)) {
		t.Promote(key, entry)
	}
}

func (t *TieredPolicy) OnInsert(key string, entry *Data) {
	t.Nursery.OnInsert(key, entry)
	if t.NurseryReaper != nil {
		t.NurseryReaper.onInsert(key, entry)
	}
}

func (t *TieredPolicy) OnDelete(key string, entry *Data) {
	switch {
	case t.Nursery.Contains(key):
		t.Nursery.OnDelete(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onDelete(key, entry)
		}
	case t.Mature.Contains(key):
		t.Mature.OnDelete(key, entry)
		t.matureSize -= entry.SizeOf()
	default:
		return
	}
}

func (t *TieredPolicy) Contains(key string) bool {
	if t.Nursery.Contains(key) || t.Mature.Contains(key) {
		return true
	}
	return false
}

func (t *TieredPolicy) SelectVictim() string {
	if t.Nursery.SelectVictim() != "" {
		return t.Nursery.SelectVictim()
	} else {
		return t.Mature.SelectVictim()
	}
}

func (t *TieredPolicy) Promote(key string, entry *Data) {
	if !t.Nursery.Contains(key) {
		return
	}
	if ok := t.Sizing(entry, t.parentCache); ok {
		t.Nursery.OnDelete(key, entry) //soft delete designed explicitly for this - we don't want to delete and reinsert into cache map
		t.Mature.OnInsert(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onDelete(key, entry)
		}
	}
}

func (t *TieredPolicy) Sizing(d *Data, c *Cache) (add bool) {
	add = true
	size := d.SizeOf()
	if size < 0 {
		t.matureSize += size
		return
	}
	if size >= t.maxMatureSize {
		add = false
		return
	}
	for size+t.matureSize >= t.maxMatureSize {
		key := t.Mature.SelectVictim()
		if data, ok := c.data[key]; ok {
			c.currentSize -= data.SizeOf()
			t.matureSize -= data.SizeOf()
			c.Delete(key)
		}
	}
	t.matureSize += size
	return
}
