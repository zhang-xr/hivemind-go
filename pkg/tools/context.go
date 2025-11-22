package tools

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
)

type Context struct {
	mu sync.RWMutex

	data map[string]interface{}

	shallowCopyKeys map[string]bool
}

func NewContext() *Context {
	return &Context{
		data:            make(map[string]interface{}),
		shallowCopyKeys: make(map[string]bool),
	}
}

func (c *Context) Set(key string, value interface{}, shallowCopy bool) {

	c.mu.Lock()

	defer c.mu.Unlock()

	c.data[key] = value
	if shallowCopy {
		c.shallowCopyKeys[key] = true
	} else {

		delete(c.shallowCopyKeys, key)
	}
}

func (c *Context) Get(key string) (interface{}, bool) {

	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

func (c *Context) GetString(key string) (string, bool) {
	val, ok := c.Get(key)
	if !ok {
		return "", false
	}
	strVal, ok := val.(string)
	return strVal, ok
}

func (c *Context) Delete(key string) {

	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	delete(c.shallowCopyKeys, key)
}

func deepCopyValue(v interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	if err := enc.Encode(&v); err != nil {
		return nil, fmt.Errorf("failed to gob encode value: %w", err)
	}

	var copiedVal interface{}

	if err := dec.Decode(&copiedVal); err != nil {
		return nil, fmt.Errorf("failed to gob decode value: %w", err)
	}
	return copiedVal, nil
}

func (c *Context) Copy() (*Context, error) {

	c.mu.RLock()
	defer c.mu.RUnlock()

	newCtx := &Context{
		data:            make(map[string]interface{}),
		shallowCopyKeys: make(map[string]bool),
	}

	for k, v := range c.shallowCopyKeys {
		newCtx.shallowCopyKeys[k] = v
	}

	for key, value := range c.data {

		if c.shallowCopyKeys[key] {

			newCtx.data[key] = value
		} else {

			copiedValue, err := deepCopyValue(value)
			if err != nil {
				return nil, fmt.Errorf("failed to deep copy value for key '%s': %w", key, err)
			}
			newCtx.data[key] = copiedValue
		}
	}

	return newCtx, nil
}
