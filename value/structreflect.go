/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// reflectStructCache keeps track of json tag related data for structs and fields to speed up reflection.
// TODO: This overlaps in functionality with the fieldCache in
// https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apimachinery/pkg/runtime/converter.go#L57 but
// is more efficient at lookup by json field name. The logic should be consolidated. Only one copy of the cache needs
// to be kept for each running process.
var (
	reflectStructCache = newStructCache()
)

type structCache struct {
	// use an atomic and copy-on-write since there are a fixed (typically very small) number of structs compiled into any
	// go program using this cache
	value atomic.Value
	// mu is held by writers when performing load/modify/store operations on the cache, readers do not need to hold a
	// read-lock since the atomic value is always read-only
	mu sync.Mutex
}

type structCacheMap map[reflect.Type]structCacheEntry

// structCacheEntry contains information about each struct field, keyed by json field name, that is expensive to
// compute using reflection.
type structCacheEntry struct {
	byJsonName map[string]*fieldCacheEntry
	fieldList  []*fieldCacheEntry // for index based iteration
}

// Get returns true and fieldCacheEntry for the given type if the type is in the cache. Otherwise Get returns false.
func (c *structCache) Get(t reflect.Type) (structCacheEntry, bool) {
	entry, ok := c.value.Load().(structCacheMap)[t]
	return entry, ok
}

// Update sets the fieldCacheEntry for the given type via a copy-on-write update to the struct cache.
func (c *structCache) Update(t reflect.Type, m structCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldCacheMap := c.value.Load().(structCacheMap)
	newCacheMap := make(structCacheMap, len(oldCacheMap)+1)
	for k, v := range oldCacheMap {
		newCacheMap[k] = v
	}
	newCacheMap[t] = m
	c.value.Store(newCacheMap)
}

func newStructCache() *structCache {
	cache := &structCache{}
	cache.value.Store(make(structCacheMap))
	return cache
}

type fieldCacheEntry struct {
	// key is the json name of the field
	key string
	// isOmitEmpty is true if the field has the json 'omitempty' tag.
	isOmitEmpty bool
	// fieldPath is the field indices (see FieldByIndex) to lookup the value of
	// a field in a reflect.Value struct. A path of field indices is used
	// to support traversing to a field nested in struct fields that have the 'inline'
	// json tag.
	fieldPath [][]int
}

func (f *fieldCacheEntry) getFieldFromStruct(structVal reflect.Value) reflect.Value {
	// field might be nested within 'inline' structs
	for _, elem := range f.fieldPath {
		structVal = structVal.FieldByIndex(elem)
	}
	return structVal
}

func getStructCacheEntry(t reflect.Type) structCacheEntry {
	if record, ok := reflectStructCache.Get(t); ok {
		return record
	}

	byJsonKey := map[string]*fieldCacheEntry{}
	buildStructCacheEntry(t, byJsonKey, nil)
	fieldList := make([]*fieldCacheEntry, len(byJsonKey))
	i := 0
	for _, v := range byJsonKey {
		fieldList[i] = v
		i++
	}
	record := structCacheEntry{byJsonKey, fieldList}
	reflectStructCache.Update(t, record)
	return record
}

func buildStructCacheEntry(t reflect.Type, byJsonName map[string]*fieldCacheEntry, fieldPath [][]int) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonName, isInline, isOmitempty := lookupJsonTags(field)
		if isInline {
			buildStructCacheEntry(field.Type, byJsonName, append(fieldPath, field.Index))
			continue
		}
		entry := &fieldCacheEntry{isOmitEmpty: isOmitempty, fieldPath: append(fieldPath, field.Index), key: jsonName}
		byJsonName[jsonName] = entry
	}
}

type structReflect struct {
	Value reflect.Value
}

func (r structReflect) Length() int {
	i := 0
	eachStructField(r.Value, func(s string, value reflect.Value) bool {
		i++
		return true
	})
	return i
}

func (r structReflect) Get(key string) (Value, bool) {
	if val, ok := r.findJsonNameField(key); ok {
		return mustWrapValueReflect(val), true
	}
	return nil, false
}

func (r structReflect) Has(key string) bool {
	_, ok := r.findJsonNameField(key)
	return ok
}

func (r structReflect) Set(key string, val Value) {
	fieldVal, ok := r.findJsonNameField(key)
	if !ok {
		panic(fmt.Sprintf("key %s may not be set on struct %T: field does not exist", key, r.Value.Interface()))
	}
	if !fieldVal.CanSet() {
		// See https://blog.golang.org/laws-of-reflection for details on why a struct may not be settable
		panic(fmt.Sprintf("key %s may not be set on struct: %T: struct is not settable", key, r.Value.Interface()))
	}
	fieldVal.Set(reflect.ValueOf(val.Unstructured()))
}

func (r structReflect) Delete(key string) {
	fieldVal, ok := r.findJsonNameField(key)
	if !ok {
		panic(fmt.Sprintf("key %s may not be deleted on struct %T: field does not exist", key, r.Value.Interface()))
	}
	if !fieldVal.CanSet() {
		// See https://blog.golang.org/laws-of-reflection for details on why a struct may not be settable
		panic(fmt.Sprintf("key %s may not be deleted on struct: %T: struct is not settable", key, r.Value.Interface()))
	}
	fieldVal.Set(reflect.Zero(fieldVal.Type()))
}

func (r structReflect) Iterate(fn func(string, Value) bool) bool {
	return eachStructField(r.Value, func(s string, value reflect.Value) bool {
		v := mustWrapValueReflect(value)
		defer v.Recycle()
		return fn(s, v)
	})
}

func newStructReflectIter(val reflect.Value) MapIter {
	fieldList := getStructCacheEntry(val.Type()).fieldList
	iter := &structReflectIter{value: val, fieldList: fieldList}
	iter.idx = -1
	return iter
}

type structReflectIter struct {
	value     reflect.Value
	fieldList []*fieldCacheEntry
	current   reflect.Value
	idx       int
}

func (i *structReflectIter) Key() string {
	return i.fieldList[i.idx].key
}

func (i *structReflectIter) Value() Value {
	return mustWrapValueReflect(i.current)
}

func (i *structReflectIter) Next() bool {
	i.idx++
	if i.idx < len(i.fieldList) {
		i.skipOmit()
	}
	return i.idx < len(i.fieldList)
}

func (i *structReflectIter) skipOmit() {
	if i.idx == -1 {
		return
	}
	for i.idx < len(i.fieldList) {
		fieldEntry := i.fieldList[i.idx]
		fieldVal := fieldEntry.getFieldFromStruct(i.value)
		if fieldEntry.isOmitEmpty {
			if safeIsNil(fieldVal) || isZero(fieldVal) {
				i.idx++
				continue
			}
		}
		i.current = fieldVal
		return
	}
}

func (r structReflect) Range() MapIter {
	iter := newStructReflectIter(r.Value)
	return iter
}

func eachStructField(structVal reflect.Value, fn func(string, reflect.Value) bool) bool {
	for jsonName, fieldCacheEntry := range getStructCacheEntry(structVal.Type()).byJsonName {
		fieldVal := fieldCacheEntry.getFieldFromStruct(structVal)
		if fieldCacheEntry.isOmitEmpty && (safeIsNil(fieldVal) || isZero(fieldVal)) {
			// omit it
			continue
		}
		ok := fn(jsonName, fieldVal)
		if !ok {
			return false
		}
	}
	return true
}

func (r structReflect) Unstructured() interface{} {
	// Use number of struct fields as a cheap way to rough estimate map size
	result := make(map[string]interface{}, r.Value.NumField())
	r.Iterate(func(s string, value Value) bool {
		result[s] = value.Unstructured()
		return true
	})
	return result
}

func (r structReflect) Equals(m Map) bool {
	if rhsStruct, ok := m.(structReflect); ok {
		return reflect.DeepEqual(r.Value.Interface(), rhsStruct.Value.Interface())
	}
	if r.Length() != m.Length() {
		return false
	}
	structCacheEntry := getStructCacheEntry(r.Value.Type()).byJsonName

	iter := m.Range()
	for iter.Next() {
		key := iter.Key()
		fieldCacheEntry, ok := structCacheEntry[key]
		if !ok {
			return false
		}
		value := iter.Value()
		lhsVal := mustWrapValueReflect(fieldCacheEntry.getFieldFromStruct(r.Value))
		equals := Equals(lhsVal, value)
		value.Recycle()
		lhsVal.Recycle()
		if !equals {
			return false
		}
	}
	return true
}

func (r structReflect) findJsonNameFieldAndNotEmpty(jsonName string) (reflect.Value, bool) {
	structCacheEntry, ok := getStructCacheEntry(r.Value.Type()).byJsonName[jsonName]
	if !ok {
		return reflect.Value{}, false
	}
	fieldVal := structCacheEntry.getFieldFromStruct(r.Value)
	omit := structCacheEntry.isOmitEmpty && (safeIsNil(fieldVal) || isZero(fieldVal))
	return fieldVal, !omit
}

func (r structReflect) findJsonNameField(jsonName string) (reflect.Value, bool) {
	structCacheEntry, ok := getStructCacheEntry(r.Value.Type()).byJsonName[jsonName]
	if !ok {
		return reflect.Value{}, false
	}
	fieldVal := structCacheEntry.getFieldFromStruct(r.Value)
	return fieldVal, true
}
