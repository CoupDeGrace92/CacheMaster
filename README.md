# CacheMasters
Designed partially as an exploration of using garbage collection/lifecycle management methods in caching,
CacheMasters lets users create caches with a simple command, then specify the architecture they want
without having to implement the inner workings themselves.

CacheMasters creates caches of `[]byte` so that users can easily store any data they want in the cache
and uses the `EvictionPolicy` interface to define how those entries are managed when the cache reaches
its maximum size. CacheMasters also uses the `TimeReap` interface to allow for any number of goroutines
that clean up cache entries based on specified criteria. For each, CacheMasters has implemented a small
set of structures that belong to those interfaces but exposes those interfaces so end users can create their own!

Ever wanted a caching policy that evicts objects that take up way too much of the cache once the cache
reaches maximum size? Well now you can! How about any data object that has more than ten m-dashes in it?
Also possible! How about evicting any cache data that was committed to the cache by Bob (we all know what his code
looks like)? A little tricky, but you can write the eviction policy for it (or in this case, since you don't want
his data to ever live in the cache, how about trying a TimeReap policy instead)!

Creating a new cache is as easy as calling `name := cache.NewCache()`. We then set the specifications of
the policy and we are off!

CacheMasters' solution to caching allows users to maintain control over most of the architectural choices without
having to deal with the implementation (at the cost of some memory overhead since the fields that account for
abstracted eviction policies take up a minimal amount of space). You can choose the eviction policy (or even
no eviction policy if you want, I am not your boss - probably).

## Installation

```bash
go get github.com/CoupDeGrace92/CacheMaster
```


## Quick Start

```go
package main

import (
    cache "github.com/CoupDeGrace92/CacheMaster/cache"
)

func main() {
    c := cache.NewCache()
    policy := cache.NewLRUPolicy()
    err := c.SetEvictionPolicy(policy)
    
    //this should only error if a policy already exists - creating a policy on already existing data 
    //with a policy will orphan data in the cache that will not be able to be evicted
    if err != nil {
        return
    }

    c.SetSize(1000000) //Size is specified in bytes

    //And the cache is set up and ready to use.  Just call c.Set(key, value), c.Get(key), or c.Delete(key)
    //for whatever cache operations you want to use.
}
```
## Example Caching with a Tiered Cache

```go
package main

import (
    "time"
    "log"

    cache "github.com/CoupDeGrace92/CacheMaster"
)

func main(){
    c := cache.NewCache()
    n := cache.NewLRUPolicy() //Usually the nursery policy should be least recently used to avoid cache polution
                        //but you can specify whatever that satisfies the EvictionPolicy interface
    m := cache.NewLFUPolicy() //Eviction policy of the mature cache can be whatever
    t := cache.NewCAReap(time.Duration(100) * time.Second) //Illustrate adding a timed reaper to the nurserey
                                        //CA stands for created at and will reap entries based on time since
                                        //creation

    policy, err := cache.NewTieredPolicy(n, m, t, time.Duration(100)*time.Millisecond, c)
    if err != nil {
        log.Println(err)
        return
    }

    err = c.SetEvictionPolicy(policy)
    if err != nil {
        log.Println(err)
        return
    }
    //and now we can start caching
}
```

## Usage
### Cache Configuration

| Method | Parameter | Type | Default | Description |
|---|---|---|---|---|
| `NewCache()` | - | - | - | Creates a new cache with no size limit and no eviction policy |
| `SetSize(i)` | `i` | `int` | `math.MaxInt` | Sets maximum cache size in bytes. Clamps to 0 if negative |
| `AddSize(i)` | `i` | `int` | - | Adds or subtracts from current max size. Clamps to 0 if negative |
| `SetEvictionPolicy(m)` | `m` | `EvictionPolicy` | `nil` | Sets eviction policy. Returns error if policy already exists |
| `AddManagedReaper(m)` | `m` | `TimeReap` | - | Adds a time-based reaper goroutine to the cache |

### Tiered Policy Configuration
This policy is based on promoting entries that survive the initial stage of their life to a longer lived portion of
the cache.  Note that promotion frequency and survival time are disjunctive and there is no current support for 
conjunctive promotion criteria.  However, since CacheMaster exposes the eviction policy interface, users that need
a conjunctive promotion selection can create their own.
| Method | Parameter | Type | Default | Description |
|---|---|---|---|---|
| `NewTieredPolicy(...)` | `Nursery, Mature` | `EvictionPolicy` | required | Eviction policies for each tier |
| | `Reaper` | `TimeReap` | `nil` | Optional time-based reaper for the nursery |
| | `reapInterval` | `time.Duration` | required if Reaper != nil | How often the nursery reaper runs |
| | `c` | `*Cache` | required | Parent cache reference |
| `SetPromotionFreq(i)` | `i` | `int` | `math.MaxInt` | Access count required for nursery promotion. Promotion is disjunctive - either threshold triggers promotion |
| `SetSurvivalTime(d)` | `d` | `time.Duration` | `1000000h` | Age required for nursery promotion |
| `SetMaxMatureSize(s)` | `s` | `int` | `math.MaxInt` | Maximum size of the mature tier in bytes |

### Time-Based Reaper Configuration

#### Last-Access Reaper (LAReap)
Evicts entries that have not been accessed within a specified duration.

| Constructor | Parameter | Type | Description |
|---|---|---|---|
| `NewLAReap(maxAge)` | `maxAge` | `time.Duration` | Maximum time since last access before an entry is reaped |

#### Created-At Reaper (CAReap)
Evicts entries that have existed in the cache longer than a specified duration, regardless of access patterns.

| Constructor | Parameter | Type | Description |
|---|---|---|---|
| `NewCAReap(maxAge)` | `maxAge` | `time.Duration` | Maximum time since creation before an entry is reaped |


### Cache Operations

| Method | Parameters | Returns | Description |
|---|---|---|---|
| `Set(key, value)` | `key string`, `value []byte` | `bool` | Adds or updates an entry. Returns `false` if the entry could not be added due to size constraints |
| `Get(key)` | `key string` | `[]byte, bool` | Retrieves an entry by key. Returns `nil, false` if the key does not exist |
| `Delete(key)` | `key string` | - | Removes an entry by key. No-op if key does not exist |
| `MakePerm(key)` | `key string` | - | Promotes an entry to permanent storage, exempt from eviction |
| `MakeNonPerm(key)` | `key string` | - | Returns a permanent entry to the evictable pool |
| `GetMaxSize()` | - | `int` | Returns the current maximum cache size in bytes |
| `GetCurrentSize()` | - | `int` | Returns the current total size of all entries in bytes |
| `GetCurrentPermSize()` | - | `int` | Returns the current size of permanent entries in bytes |

### Discussions of EvictionPolicy and TimeReap
#### EvictionPolicy
The EvictionPolicy interface defines how the cache selects entries for removal when the cache reaches maximum size:

```go
type EvictionPolicy interface {
    onInsert(key string, entry *Data)
    onAccess(key string, entry *Data)
    onDelete(key string, entry *Data)
    contains(key string) bool
    selectVictim() string
}
```

CacheMaster provides two built in implementations:
    - `NewLRUPolicy()` - evicts the least recently used entry
    - `NewLFUPolicy()` - evicts the least frequently used entry

Note in many caching scenarios, a LFU cache can have issues with "cache pollution" whereby new entries to the cache
will be more likely to be evicted than older entries that have been incredibly infrequently used but for some reason
had their frequency boosted at some point.  Thus, it is suggested not to use this for the nursery cache in tiered caching
systems and for non-tiered caching systems, use this in conjunction with some kind of timed reaper so that polluting
entries can have a different method for leaving the cache.

Recency based eviction, on the other hand, subjects entries to a higher degree of randomness - over a long time 
horizon, frequency and recency should generally pick similar entries but in the short term, what entries we see
most recently can best be thought of as what statisticians think of as a random error - unpredictable chance variability
in data.

If neither these, nor a tiered policy fit your usecase, you can create a custom policy by implementing the EvictionPolicy
interface.  onAccess and onInsert are called on every cache access and insertion respectively, allowing the policy to maintain
the internal state it needs to make an eviction decision.  If necessary, you can wrap the data structure in a 
struct to keep a custom state necessary for your implementation.  Just like in CacheMaster, additional state fields
do have overhead on data size in the cache (one of the trade offs for ease of use with CacheMaster).

selectVictim() is called when the cache needs to free space and should return the key of the entry to evict. Creating
a custom interface is demonstrated below with RandomPolicy.  Methods with no relevant behavior can be left as no-ops 
(see onAccess for RandomPolicy illustrated below - on access does not care about data state so onAccess does not update
any states relevant to the data in question).

#### TimeReap
The TimeReap interface defines a goroutine-based cleanup loop that periodically removes entries based on time-based criteria:
```go
type TimeReap interface{
    onInsert(key string, entry *Data)
    onAccess(key string, entry *Data)
    onDelete(key string, entry *Data)
    Reap(interval time.Duration, cache *Cache) chan struct{}
}
```

CacheMaster provides two implementations:
    - NewLAReap(maxAge time.Duration) - evicts entries that have not been accessed within maxAge
    - NewCAReap(maxAge time.Duration) - evicts entries that have existed in the cache longer than maxAge

The Reap method is responsible for starting the cleanup goroutine and returning a stop channel that can be used to
terminate the goroutine if desired.


### Example of the EvictionPolicy interface
For a simple example - we will construct an eviction policy that evicts a random entry when called.  Utilizing
the fact that map access is pseudorandom:

```go
type RandomPolicy struct {
    keys map[string]struct{}
}

func NewRandomPolicy() *RandomPolicy {
    return &RandomPolicy{
        keys: make(map[string]struct{}),
    }
}

func (r *RandomPolicy) onInsert(key string, entry *Data) {
    r.keys[key] = struct{}{}
}

func (r *RandomPolicy) onAccess(key string, entry *Data) {}

func (r *RandomPolicy) onDelete(key string, entry *Data) {
    delete(r.keys, key)
}

func (r *RandomPolicy) contains(key string) bool {
    _, ok := r.keys[key]
    return ok
}

func (r *RandomPolicy) selectVictim() string {
    for key := range r.keys {
        return key
    }
    return ""
}
```

For custom policies that need some sort of metadata not contained within the Data struct, we have two
recommendations BUT as always, you are in control and any solution that you land on that fits your usecase
works.  CacheMaster suggests using either an external metadata map OR a wrapper struct used as the byte slice:

#### External metadata map
```go
type MyMetadata struct {
    //Metadata fields here - e.g. tag
    key      string
    tag      string
}

metaMap := make(map[string]*MyMetadata)
```

#### Wrapper struct used as a byte slice
```go
type DataPlus struct {
    //Metadata fields here - e.g. tag
    tag      string
    Payload  []byte
}
// serialize to []byte before Set, deserialize after Get
```

Using an external metadata map is suggested for general use cases but a wrapper structure is useful to keep
all data in one place (it will exist on the data structure within the caches internal map instead of on an
additional map)

### The Data Structure

Custom `EvictionPolicy` and `TimeReap` implementations have access to the following fields on the `Data` struct:

| Field | Type | Description |
|---|---|---|
| `CreatedAt` | `time.Time` | The time the entry was first inserted into the cache |
| `LastAccess` | `time.Time` | The time the entry was most recently accessed via `Get` or updated via `Set` |
| `Count` | `int` | The number of times the entry has been accessed or inserted |

Note that `Count` is incremented on both insertion and access. A freshly inserted entry will have a `Count` of `1`.

Direct modification of these fields is supported within custom policy implementations but modifying them outside of a policy context may result in unexpected eviction behavior. The underlying data and permanence of an entry are managed exclusively through `Set`, `Get`, `Delete`, `MakePerm`, and `MakeNonPerm`.

## Contributing

To contribute - first clone the repo:
```bash
git clone https://github.com/CoupDeGrace92/CacheMaster
```

Any feature changes you are interested in implementing should be checked out into an appropriate git branch.

```bash
git checkout -b <branch-name-for-change>
```

Then once you have made changes, add any tests to a testify file that are consistent with the changes (that illustrate
the bug that currently exists OR show off the intended behavior of the desired feature as well as tests on edge cases)

The command for testing the repo:
```bash
go test ./...
```

Also make sure the binary compiles:
```bash
go build
```

If you are satisfied, feel free to submit the request.

```bash
git pull origin main https://github.com/CoupDeGrace92/CacheMaster
```