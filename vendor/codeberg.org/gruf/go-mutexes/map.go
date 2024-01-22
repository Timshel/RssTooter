package mutexes

import (
	"sync"
	"sync/atomic"
)

const (
	// possible lock types.
	lockTypeRead  = uint8(1) << 0
	lockTypeWrite = uint8(1) << 1

	// frequency of GC cycles
	// per no. unlocks. i.e.
	// every 'gcfreq' unlocks.
	gcfreq = 1024
)

// MutexMap is a structure that allows read / write locking
// per key, performing as you'd expect a map[string]*RWMutex
// to perform, without you needing to worry about deadlocks
// between competing read / write locks and the map's own mutex.
// It uses memory pooling for the internal "mutex" (ish) types
// and performs self-eviction of keys.
//
// Under the hood this is achieved using a single mutex for the
// map, state tracking for individual keys, and some sync.Cond{}
// like structures for sleeping / awaking awaiting goroutines.
type MutexMap struct {
	mapmu  sync.Mutex
	mumap  map[string]*rwmutex
	mupool rwmutexPool
	count  uint32
}

// checkInit ensures MutexMap is initialized (UNSAFE).
func (mm *MutexMap) checkInit() {
	if mm.mumap == nil {
		mm.mumap = make(map[string]*rwmutex)
	}
}

// Lock acquires a write lock on key in map, returning unlock function.
func (mm *MutexMap) Lock(key string) func() {
	return mm.lock(key, lockTypeWrite)
}

// RLock acquires a read lock on key in map, returning runlock function.
func (mm *MutexMap) RLock(key string) func() {
	return mm.lock(key, lockTypeRead)
}

func (mm *MutexMap) lock(key string, lt uint8) func() {
	// Perform first map lock
	// and check initialization
	// OUTSIDE the main loop.
	mm.mapmu.Lock()
	mm.checkInit()

	for {
		// Check map for mu.
		mu := mm.mumap[key]

		if mu == nil {
			// Allocate new mutex.
			mu = mm.mupool.Acquire()
			mm.mumap[key] = mu
		}

		if !mu.Lock(lt) {
			// Wait on mutex unlock, after
			// immediately relocking map mu.
			mu.WaitRelock(&mm.mapmu)
			continue
		}

		// Done with map.
		mm.mapmu.Unlock()

		// Return mutex unlock function.
		return func() { mm.unlock(key, mu) }
	}
}

func (mm *MutexMap) unlock(key string, mu *rwmutex) {
	// Get map lock.
	mm.mapmu.Lock()

	// Unlock mutex.
	if mu.Unlock() {

		// Mutex fully unlocked
		// with zero waiters. Self
		// evict and release it.
		delete(mm.mumap, key)
		mm.mupool.Release(mu)
	}

	if mm.count++; mm.count%gcfreq == 0 {
		// Every 'gcfreq' unlocks perform
		// a garbage collection to keep
		// us squeaky clean :]
		mm.mupool.GC()
	}

	// Done with map.
	mm.mapmu.Unlock()
}

// rwmutexPool is a very simply memory rwmutexPool.
type rwmutexPool struct {
	current []*rwmutex
	victim  []*rwmutex
}

// Acquire will returns a rwmutexState from rwmutexPool (or alloc new).
func (p *rwmutexPool) Acquire() *rwmutex {
	// First try the current queue
	if l := len(p.current) - 1; l >= 0 {
		mu := p.current[l]
		p.current = p.current[:l]
		return mu
	}

	// Next try the victim queue.
	if l := len(p.victim) - 1; l >= 0 {
		mu := p.victim[l]
		p.victim = p.victim[:l]
		return mu
	}

	// Lastly, alloc new.
	mu := new(rwmutex)
	return mu
}

// Release places a sync.rwmutexState back in the rwmutexPool.
func (p *rwmutexPool) Release(mu *rwmutex) {
	p.current = append(p.current, mu)
}

// GC will clear out unused entries from the rwmutexPool.
func (p *rwmutexPool) GC() {
	current := p.current
	p.current = nil
	p.victim = current
}

// rwmutex represents a RW mutex when used correctly within
// a MapMutex. It should ONLY be access when protected by
// the outer map lock, except for the 'notifyList' which is
// a runtime internal structure borrowed from the sync.Cond{}.
//
// this functions very similarly to a sync.Cond{}, but with
// lock state tracking, and returning on 'Broadcast()' whether
// any goroutines were actually awoken. it also has a less
// confusing API than sync.Cond{} with the outer locking
// mechanism we use, otherwise all Cond{}.L would reference
// the same outer map mutex.
type rwmutex struct {
	n notifyList // 'trigger' mechanism
	l int32      // no. locks
	t uint8      // lock type
}

// Lock will lock the mutex for given lock type, in the
// sense that it will update the internal state tracker
// accordingly. Return value is true on successful lock.
func (mu *rwmutex) Lock(lt uint8) bool {
	switch mu.t {
	case lockTypeRead:
		// already read locked,
		// only permit more reads.
		if lt != lockTypeRead {
			return false
		}

	case lockTypeWrite:
		// already write locked,
		// no other locks allowed.
		return false

	default:
		// Fully unlocked,
		// set incoming type.
		mu.t = lt
	}

	// Update
	// count.
	mu.l++

	return true
}

// Unlock will unlock the mutex, in the sense that it
// will update the internal state tracker accordingly.
// On totally unlocked state, it will awaken all
// sleeping goroutines waiting on this mutex.
func (mu *rwmutex) Unlock() bool {
	switch mu.l--; {
	case mu.l > 0 && mu.t == lockTypeWrite:
		panic("BUG: multiple writer locks")
	case mu.l < 0:
		panic("BUG: negative lock count")

	case mu.l == 0:
		// Fully unlocked.
		mu.t = 0

		// Awake all blocked goroutines and check
		// for change in the last notified ticket.
		before := atomic.LoadUint32(&mu.n.notify)
		runtime_notifyListNotifyAll(&mu.n)
		after := atomic.LoadUint32(&mu.n.notify)

		// If ticket changed, this indicates
		// AT LEAST one goroutine was awoken.
		//
		// (before != after) => (waiters > 0)
		// (before == after) => (waiters = 0)
		return (before == after)

	default:
		// i.e. mutex still
		// locked by others.
		return false
	}
}

// WaitRelock expects a mutex to be passed in, already in the
// locked state. It incr the notifyList waiter count before
// unlocking the outer mutex and blocking on notifyList wait.
// On awake it will decr wait count and relock outer mutex.
func (mu *rwmutex) WaitRelock(outer *sync.Mutex) {

	// add ourselves to list while still
	// under protection of outer map lock.
	t := runtime_notifyListAdd(&mu.n)

	// Finished with
	// outer map lock.
	outer.Unlock()

	// Block until awoken by another
	// goroutine within mu.Unlock().
	runtime_notifyListWait(&mu.n, t)

	// Relock!
	outer.Lock()
}
