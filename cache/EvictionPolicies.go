package cache

import (
	"math"
	"time"
)

// Time based cache eviction
// Create linked list, reap tail until we find object not ready for reaping, then wait ticker
func (c *Cache) LastAccessReap(interval time.Duration) {
}

// we can be more efficient here if we organize it into a linked list - inserted at creation
func (c *Cache) TimeSinceCreationReap(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for t := range ticker.C {
		for key, entry := range c.data {
			delTime := entry.CreatedAt.Add(interval)
			if delTime.Compare(t) == -1 {
				c.Delete(key)
			}
		}
	}
}

type EvictionPolicy interface {
	OnInsert(key string, entry *Data)
	OnAccess(key string, entry *Data)
	OnDelete(key string, entry *Data)
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
	entry.CreatedAt = time.Now()
	entry.LastAccess = time.Now()
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
	entry.Count++
	entry.LastAccess = time.Now()
	prev := p.promoteRemove(entry, node)
	p.addToBucket(node, prev, entry)
}

func (p *LFUPolicy) OnDelete(key string, entry *Data) {
	node := p.nodeMap[key]
	p.removeFromBucket(entry, node)
	delete(p.nodeMap, key)
}

func (p *LFUPolicy) SelectVictim() (key string) {
	key = p.removeFromBucketTail()
	delete(p.nodeMap, key)
	return
}

// Size based cache eviction
func OldestEntry() {

}

func ReapLargest() {

}

// More Complex Solutions
func CacheIncubation() {

}

func WeightedEviction() {

}
