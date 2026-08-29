// Package buffer implements a small LRU page cache.
//
// It sits between the B+ tree / heap layers and storage.PageManager,
// caching pages in memory and deferring writes until a dirty page is
// evicted or the pool is closed. The pool also tracks cache and flush
// statistics so page I/O can be measured directly.
package buffer

import (
	"container/list"

	"github.com/DanielPopoola/index_lab/internal/page"
	"github.com/DanielPopoola/index_lab/internal/storage"
)

// entry is a cached page together with a dirty flag.
//
// dirty is set explicitly by MarkDirty because the pool cannot observe
// mutations made to the page after it is returned to a caller.
type entry struct {
	pageID page.PageID
	page   *page.Page
	dirty  bool
}

// Pool is a fixed-capacity page cache backed by a storage.PageManager.
//
// When full, the least-recently-used page is evicted to make room for a
// new one. Dirty pages are written back before eviction.
type Pool struct {
	pm       *storage.PageManager
	capacity int

	// items maps a PageID to its node in lru, so a cached page can be
	// found and promoted to most-recently-used in O(1).
	items map[page.PageID]*list.Element

	// lru orders cached pages by recency. Front = most recently used.
	// Back = least recently used, i.e. the next eviction candidate.
	lru *list.List

	// stats
	hits         uint64
	misses       uint64
	evictions    uint64
	dirtyFlushes uint64
}

// NewPool returns a buffer pool of the given capacity in front of pm.
//
// capacity must be at least 1.
func NewPool(pm *storage.PageManager, capacity int) *Pool {
	if capacity < 1 {
		capacity = 1
	}
	return &Pool{
		pm:       pm,
		capacity: capacity,
		items:    make(map[page.PageID]*list.Element),
		lru:      list.New(),
	}
}

// GetPage returns the page for id from the cache if present; otherwise it
// reads the page from the backing PageManager and caches it.
func (p *Pool) GetPage(id page.PageID) (*page.Page, error) {
	if elem, ok := p.items[id]; ok {
		p.lru.MoveToFront(elem)
		p.hits++
		return elem.Value.(*entry).page, nil
	}

	p.misses++
	pg, err := p.pm.ReadPage(id)
	if err != nil {
		return nil, err
	}
	if err := p.evictIfFull(); err != nil {
		return nil, err
	}

	entry := &entry{pageID: id, page: pg}
	p.items[id] = p.lru.PushFront(entry)
	return pg, nil
}

// Put caches pg under id without reading it from disk first.
//
// This is used for freshly allocated pages that have no disk image yet but
// still need to be retained in memory and flushed eventually. If id is
// already cached, its entry is replaced.
func (p *Pool) Put(id page.PageID, pg *page.Page) error {
	if elem, ok := p.items[id]; ok {
		e := elem.Value.(*entry)
		e.page = pg
		e.dirty = true
		p.lru.MoveToFront(elem)
		return nil
	}

	if err := p.evictIfFull(); err != nil {
		return err
	}

	e := &entry{pageID: id, page: pg, dirty: true}
	p.items[id] = p.lru.PushFront(e)
	return nil
}

// MarkDirty marks the cached page id as modified.
//
// The caller should call it immediately after mutating a page returned by
// GetPage. If id is not cached, MarkDirty is a no-op.
func (p *Pool) MarkDirty(id page.PageID) {
	elem, ok := p.items[id]
	if !ok {
		return
	}
	e, ok := elem.Value.(*entry)
	if !ok {
		return
	}
	e.dirty = true
}

// evictIfFull evicts the least-recently-used page when the cache is full.
//
// A dirty page is written back before it is removed from the cache.
func (p *Pool) evictIfFull() error {
	if len(p.items) < p.capacity {
		return nil
	}
	if len(p.items) == 0 {
		return nil
	}

	elem := p.lru.Back()
	if elem == nil {
		return nil
	}
	entry, ok := elem.Value.(*entry)
	if !ok {
		return nil
	}
	if entry.dirty {
		if err := p.pm.WritePage(entry.page); err != nil {
			return err
		}
		p.dirtyFlushes++
	}

	p.lru.Remove(elem)
	delete(p.items, entry.pageID)
	p.evictions++
	return nil
}

// FlushPage writes the cached page id back to disk if it is dirty and clears
// the dirty flag.
//
// It is a no-op if id is not cached or the page is already clean.
func (p *Pool) FlushPage(id page.PageID) error {
	elem, ok := p.items[id]
	if !ok {
		return nil
	}
	e, ok := elem.Value.(*entry)
	if !ok || !e.dirty {
		return nil
	}
	if err := p.pm.WritePage(e.page); err != nil {
		return err
	}
	p.dirtyFlushes++
	e.dirty = false
	return nil
}

// Close flushes all dirty pages in the cache and then closes the underlying
// PageManager.
func (p *Pool) Close() error {
	for id := range p.items {
		if err := p.FlushPage(id); err != nil {
			_ = p.pm.Close()
			return err
		}
	}
	return p.pm.Close()
}

// CacheHits returns the number of GetPage calls served from the cache.
func (p *Pool) CacheHits() uint64 {
	return p.hits
}

// CacheMisses returns the number of GetPage calls that required a disk read.
func (p *Pool) CacheMisses() uint64 {
	return p.misses
}

// Evictions returns the number of pages evicted from the cache.
func (p *Pool) Evictions() uint64 {
	return p.evictions
}

// DirtyFlushes returns the number of dirty-page writes performed so far.
func (p *Pool) DirtyFlushes() uint64 {
	return p.dirtyFlushes
}

// ResetStats clears the hit, miss, eviction, and flush counters.
//
// It does not affect the cache contents.
func (p *Pool) ResetStats() {
	p.hits = 0
	p.misses = 0
	p.evictions = 0
	p.dirtyFlushes = 0
}
