/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package value

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

type UnstructuredStringConvertable interface {
	ToUnstructuredString() (string, bool)
}

func GetReflectCacheEntry(t reflect.Type) ReflectCacheEntry {
	if record, ok := defaultReflectCache.get(t); ok {
		return record
	}
	record := ReflectCacheEntry{
		isJsonMarshaler:    t.Implements(marshalerType),
		ptrIsJsonMarshaler: reflect.PtrTo(t).Implements(marshalerType),
		isStringConvertable:    t.Implements(unstructuredConvertableType),
		ptrIsStringConvertable: reflect.PtrTo(t).Implements(unstructuredConvertableType),
	}
	defaultReflectCache.update(t, record)
	return record
}

type ReflectCacheEntry struct {
	isJsonMarshaler     bool
	ptrIsJsonMarshaler  bool
	isStringConvertable bool
	ptrIsStringConvertable bool
}

func (e ReflectCacheEntry) CanConvert() bool {
	return e.isJsonMarshaler || e.ptrIsJsonMarshaler || e.isStringConvertable || e.ptrIsStringConvertable
}

func (e ReflectCacheEntry) FromUnstructured(sv, dv reflect.Value) error {
	st := sv.Type()
	data, err := json.Marshal(sv.Interface())
	if err != nil {
		return fmt.Errorf("error encoding %s to json: %v", st.String(), err)
	}
	unmarshaler := dv.Addr().Interface().(json.Unmarshaler)
	return unmarshaler.UnmarshalJSON(data)
}

func (e ReflectCacheEntry) ToUnstructured(sv reflect.Value) (interface{}, error) {
	// Check if the object has a custom string converter and use it if available, since it is much more efficient
	// than round tripping through json.
	entry := GetReflectCacheEntry(sv.Type())
	if converter, ok := entry.getStringConvertable(sv); ok {
		if s, ok := converter.ToUnstructuredString(); ok {
			return s, nil
		}
		return nil, nil
	}
	// Check if the object has a custom JSON marshaller/unmarshaller.
	if marshaler, ok := entry.getJsonMarshaler(sv); ok {
		if sv.Kind() == reflect.Ptr && sv.IsNil() {
			// We're done - we don't need to store anything.
			return nil, nil
		}

		data, err := marshaler.MarshalJSON()
		if err != nil {
			return nil, err
		}
		switch {
		case len(data) == 0:
			return nil, fmt.Errorf("error decoding from json: empty value")

		case bytes.Equal(data, nullBytes):
			// We're done - we don't need to store anything.
			return nil, nil

		case bytes.Equal(data, trueBytes):
			return true, nil

		case bytes.Equal(data, falseBytes):
			return false, nil

		case data[0] == '"':
			var result string
			err := json.Unmarshal(data, &result)
			if err != nil {
				return nil, fmt.Errorf("error decoding string from json: %v", err)
			}
			return result, nil

		case data[0] == '{':
			result := make(map[string]interface{})
			err := json.Unmarshal(data, &result)
			if err != nil {
				return nil, fmt.Errorf("error decoding object from json: %v", err)
			}
			return result, nil

		case data[0] == '[':
			result := make([]interface{}, 0)
			err := json.Unmarshal(data, &result)
			if err != nil {
				return nil, fmt.Errorf("error decoding array from json: %v", err)
			}
			return result, nil

		default:
			var (
				resultInt   int64
				resultFloat float64
				err         error
			)
			if err = json.Unmarshal(data, &resultInt); err == nil {
				return resultInt, nil
			} else if err = json.Unmarshal(data, &resultFloat); err == nil {
				return resultFloat, nil
			} else {
				return nil, fmt.Errorf("error decoding number from json: %v", err)
			}
		}
	}

	st := sv.Type()
	return nil, fmt.Errorf("unsupported type: %v", st.Kind())
}

var (
	nullBytes  = []byte("null")
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
)

func (e ReflectCacheEntry) getJsonMarshaler(v reflect.Value) (json.Marshaler, bool) {
	if e.isJsonMarshaler {
		return v.Interface().(json.Marshaler), true
	}
	if e.ptrIsJsonMarshaler {
		// Check pointer receivers if v is not a pointer
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			return v.Interface().(json.Marshaler), true
		}
	}
	return nil, false
}

func (e ReflectCacheEntry) getStringConvertable(v reflect.Value) (UnstructuredStringConvertable, bool) {
	if e.isStringConvertable {
		return v.Interface().(UnstructuredStringConvertable), true
	}
	if e.ptrIsStringConvertable {
		// Check pointer receivers if v is not a pointer
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			return v.Interface().(UnstructuredStringConvertable), true
		}
	}
	return nil, false
}

var marshalerType = reflect.TypeOf(new(json.Marshaler)).Elem()
var unstructuredConvertableType = reflect.TypeOf(new(UnstructuredStringConvertable)).Elem()
var defaultReflectCache = newReflectCache()

type reflectCache struct {
	// use an atomic and copy-on-write since there are a fixed (typically very small) number of structs compiled into any
	// go program using this cache
	value atomic.Value
	// mu is held by writers when performing load/modify/store operations on the cache, readers do not need to hold a
	// read-lock since the atomic value is always read-only
	mu sync.Mutex
}

func newReflectCache() *reflectCache {
	cache := &reflectCache{}
	cache.value.Store(make(reflectCacheMap))
	return cache
}

type reflectCacheMap map[reflect.Type]ReflectCacheEntry

// get returns true and ReflectCacheEntry for the given type if the type is in the cache. Otherwise get returns false.
func (c *reflectCache) get(t reflect.Type) (ReflectCacheEntry, bool) {
	entry, ok := c.value.Load().(reflectCacheMap)[t]
	return entry, ok
}

// update sets the ReflectCacheEntry for the given type via a copy-on-write update to the struct cache.
func (c *reflectCache) update(t reflect.Type, m ReflectCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldCacheMap := c.value.Load().(reflectCacheMap)
	newCacheMap := make(reflectCacheMap, len(oldCacheMap)+1)
	for k, v := range oldCacheMap {
		newCacheMap[k] = v
	}
	newCacheMap[t] = m
	c.value.Store(newCacheMap)
}
