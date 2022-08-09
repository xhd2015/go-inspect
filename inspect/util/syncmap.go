package util

import "sync"

type SyncMap struct {
	sync.Map
}

func (c *SyncMap) LoadOrCompute(key interface{}, fn func() interface{}) interface{} {
	v, _ := c.Map.LoadOrStore(key, NewOnce(fn))
	return v.(*onceFn).ensureInit()
}
func (c *SyncMap) RangeComputed(fn func(key interface{}, val interface{}) bool) {
	c.Map.Range(func(key, v interface{}) bool {
		return fn(key, v.(*onceFn).ensureInit())
	})
}

type onceFn struct {
	fn   func() interface{}
	once sync.Once

	v interface{}
}

func NewOnce(fn func() interface{}) *onceFn {
	return &onceFn{fn: fn}
}
func (c *onceFn) ensureInit() interface{} {
	c.once.Do(func() {
		c.v = c.fn()
	})
	return c.v
}
