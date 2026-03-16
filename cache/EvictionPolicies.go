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
	onInsert(key string, entry *Data)
	onAccess(key string, entry *Data)
	onDelete(key string, entry *Data)
	contains(key string) bool
	selectVictim() string
	//think about adding OnUpdate
}

// THE FIRST POLICY - LRU
type lruPolicy struct {
	head    *llNode
	tail    *llNode
	nodeMap map[string]*llNode
}

type llNode struct {
	key  string
	prev *llNode
	next *llNode
}

func NewLRUPolicy() *lruPolicy {
	return &lruPolicy{
		nodeMap: make(map[string]*llNode),
	}
}

func (p *lruPolicy) contains(key string) bool {
	_, ok := p.nodeMap[key]
	return ok
}

func (p *lruPolicy) onInsert(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		node = &llNode{
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

func (p *lruPolicy) onAccess(key string, entry *Data) {
	node := p.nodeMap[key]
	entry.Count++
	entry.LastAccess = time.Now()
	p.moveToHead(node)
}

func (p *lruPolicy) onDelete(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		return
	}
	p.removeNode(node)
	delete(p.nodeMap, key)
}

func (p *lruPolicy) selectVictim() (key string) {
	if p.tail == nil {
		return ""
	}
	key = p.tail.key
	return
}

func (p *lruPolicy) moveToHead(c *llNode) {
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

func (p *lruPolicy) removeNode(c *llNode) {
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
type lfuPolicy struct {
	buckets map[int]*Bucket
	nodeMap map[string]*llNode
	minFreq int
}

type Bucket struct {
	head *llNode
	tail *llNode
	next *Bucket
	prev *Bucket
	id   int
}

func NewLFUPolicy() *lfuPolicy {
	return &lfuPolicy{
		minFreq: 0,
		buckets: make(map[int]*Bucket),
		nodeMap: make(map[string]*llNode),
	}
}

func (p *lfuPolicy) contains(key string) bool {
	_, ok := p.nodeMap[key]
	return ok
}

func (p *lfuPolicy) addToBucket(node *llNode, prevBucket *Bucket, entry *Data) {
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

func (p *lfuPolicy) removeFromBucketTail() (key string) {
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

func (p *lfuPolicy) removeFromBucket(entry *Data, node *llNode) (previous *Bucket) {
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

func (p *lfuPolicy) promoteRemove(entry *Data, node *llNode) (previous *Bucket) {
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
func (p *lfuPolicy) onInsert(key string, entry *Data) {
	entry.LastAccess = time.Now()
	if entry.Count > 1 {
		p.OnInsertGeneric(key, entry)
		return
	}
	entry.CreatedAt = time.Now()
	entry.Count = 1
	node := &llNode{
		key: key,
	}
	p.nodeMap[key] = node
	p.addToBucket(node, nil, entry)
}

func (p *lfuPolicy) OnInsertGeneric(key string, entry *Data) {
	entry.LastAccess = time.Now()
	entry.Count++

	//We can speed this up by calling the generic method if the count is just one.
	if entry.Count == 1 {
		p.onInsert(key, entry)
		return
	}
	node := &llNode{
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

func (p *lfuPolicy) onAccess(key string, entry *Data) {
	node := p.nodeMap[key]
	entry.LastAccess = time.Now()
	prev := p.promoteRemove(entry, node)
	entry.Count++
	p.addToBucket(node, prev, entry)
}

func (p *lfuPolicy) onDelete(key string, entry *Data) {
	node, ok := p.nodeMap[key]
	if !ok {
		return
	}
	p.removeFromBucket(entry, node)
	delete(p.nodeMap, key)
}

func (p *lfuPolicy) selectVictim() (key string) {
	key = p.removeFromBucketTail()
	delete(p.nodeMap, key)
	return
}

// TIME BASED REAP POLICIES
// Last Access reap
type laReap struct {
	head   *llNode
	tail   *llNode
	m      map[string]*llNode
	maxAge time.Duration
}

func NewLAReap(maxAge time.Duration) *laReap {
	return &laReap{
		m:      make(map[string]*llNode),
		maxAge: maxAge,
	}
}

func (l *laReap) onAccess(key string, entry *Data) {
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

func (l *laReap) onInsert(key string, entry *Data) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.LastAccess = time.Now()

	node := &llNode{
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

func (l *laReap) onDelete(key string, entry *Data) {
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

func (l *laReap) Reap(interval time.Duration, cache *Cache) chan struct{} {
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

func (l *laReap) Check(maxAge time.Duration, cache *Cache) {
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
			cache.deleteNoLock(key)
		} else {
			break
		}
	}
}

// Time Since Created reap
type caReap struct {
	head   *llNode
	tail   *llNode
	m      map[string]*llNode
	maxAge time.Duration
}

func NewCAReap(maxAge time.Duration) *caReap {
	return &caReap{
		m:      make(map[string]*llNode),
		maxAge: maxAge,
	}
}

func (c *caReap) onAccess(key string, entry *Data) {
	entry.LastAccess = time.Now()
}

func (c *caReap) onInsert(key string, entry *Data) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.LastAccess = time.Now()
	node := &llNode{
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

func (c *caReap) onDelete(key string, entry *Data) {
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

func (c *caReap) Reap(interval time.Duration, cache *Cache) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				c.check(c.maxAge, cache)
			}
		}
	}()

	return stopChan
}

func (c *caReap) check(maxAge time.Duration, cache *Cache) {
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
			cache.deleteNoLock(key)
		} else {
			break
		}
	}
}

// Incubating New Cache ELements
type tieredPolicy struct {
	Nursery       EvictionPolicy
	NurseryReaper TimeReap
	Mature        EvictionPolicy
	sTime         time.Duration
	pFreq         int
	matureSize    int
	maxMatureSize int
	parentCache   *Cache
}

func NewTieredPolicy(Nursery, Mature EvictionPolicy, Reaper TimeReap, reapInterval time.Duration, c *Cache) (*tieredPolicy, error) {
	if Nursery == nil || Mature == nil || c == nil {
		return nil, fmt.Errorf("Nursery, Mature, and parent cache can not be nil")
	}

	if Reaper != nil && reapInterval == time.Duration(0) {
		return nil, fmt.Errorf("Reaper must have a reap interval")
	}
	t := &tieredPolicy{
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

func (t *tieredPolicy) SetPromotionFreq(i int) {
	t.pFreq = i
}

func (t *tieredPolicy) SetSurvivalTime(d time.Duration) {
	t.sTime = d
}

func (t *tieredPolicy) SetMaxMatureSize(s int) {
	t.maxMatureSize = s
}

func (t *tieredPolicy) onAccess(key string, entry *Data) {
	switch {
	case t.Nursery.contains(key):
		t.Nursery.onAccess(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onAccess(key, entry)
		}
		if entry.Count >= t.pFreq || time.Now().After(entry.CreatedAt.Add(t.sTime)) {
			t.promote(key, entry)
		}
	case t.Mature.contains(key):
		t.Mature.onAccess(key, entry)
	default:
	}

}

func (t *tieredPolicy) onInsert(key string, entry *Data) {
	t.Nursery.onInsert(key, entry)
	if t.NurseryReaper != nil {
		t.NurseryReaper.onInsert(key, entry)
	}
}

func (t *tieredPolicy) onDelete(key string, entry *Data) {
	switch {
	case t.Nursery.contains(key):
		t.Nursery.onDelete(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onDelete(key, entry)
		}
	case t.Mature.contains(key):
		t.Mature.onDelete(key, entry)
		t.matureSize -= entry.sizeOf()
	default:
		return
	}
}

func (t *tieredPolicy) contains(key string) bool {
	if t.Nursery.contains(key) || t.Mature.contains(key) {
		return true
	}
	return false
}

func (t *tieredPolicy) selectVictim() string {
	if t.Nursery.selectVictim() != "" {
		return t.Nursery.selectVictim()
	} else {
		return t.Mature.selectVictim()
	}
}

func (t *tieredPolicy) promote(key string, entry *Data) {
	if !t.Nursery.contains(key) {
		return
	}
	if ok := t.sizing(entry, t.parentCache); ok {
		t.Nursery.onDelete(key, entry) //soft delete designed explicitly for this - we don't want to delete and reinsert into cache map
		t.Mature.onInsert(key, entry)
		if t.NurseryReaper != nil {
			t.NurseryReaper.onDelete(key, entry)
		}
	}
}

func (t *tieredPolicy) sizing(d *Data, c *Cache) (add bool) {
	add = true
	size := d.sizeOf()
	if size < 0 {
		t.matureSize += size
		return
	}
	if size >= t.maxMatureSize {
		add = false
		return
	}
	for size+t.matureSize >= t.maxMatureSize {
		key := t.Mature.selectVictim()
		if _, ok := c.data[key]; ok {
			c.deleteNoLock(key)
		}
	}
	t.matureSize += size
	return
}
