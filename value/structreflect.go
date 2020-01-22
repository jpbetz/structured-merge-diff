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
)

type structReflect struct {
	valueReflect
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
	if val, ok, _ := r.findJsonNameField(key); ok {
		return mustWrapValueReflect(val), true
	}
	return nil, false
}

func (r structReflect) Has(key string) bool {
	_, ok, _ := r.findJsonNameField(key)
	return ok
}

func (r structReflect) Set(key string, val Value) {
	fieldEntry, ok := TypeReflectEntryOf(r.Value.Type()).Fields()[key]
	if !ok {
		panic(fmt.Sprintf("key %s may not be set on struct %T: field does not exist", key, r.Value.Interface()))
	}
	oldVal := fieldEntry.GetFrom(r.Value)
	newVal := reflect.ValueOf(val.Unstructured())
	r.update(fieldEntry, key, oldVal, newVal)
}

func (r structReflect) Delete(key string) {
	fieldEntry, ok := TypeReflectEntryOf(r.Value.Type()).Fields()[key]
	if !ok {
		panic(fmt.Sprintf("key %s may not be deleted on struct %T: field does not exist", key, r.Value.Interface()))
	}
	oldVal := fieldEntry.GetFrom(r.Value)
	if oldVal.Kind() != reflect.Ptr && !fieldEntry.isOmitEmpty {
		panic(fmt.Sprintf("key %s may not be deleted on struct: %T: value is neither a pointer nor an omitempty field", key, r.Value.Interface()))
	}
	r.update(fieldEntry, key, oldVal, reflect.Zero(oldVal.Type()))
}

func (r structReflect) update(fieldEntry *FieldCacheEntry, key string, oldVal, newVal reflect.Value) {
	if oldVal.CanSet() {
		oldVal.Set(newVal)
		return
	}

	// map items are not addressable, so if a struct is contained in a map, the only way to modify it is
	// to write a replacement fieldEntry into the map.
	if r.ParentMap != nil {
		if r.ParentMapKey == nil {
			panic("ParentMapKey must not be nil if ParentMap is not nil")
		}
		replacement := reflect.New(r.Value.Type()).Elem()
		fieldEntry.GetFrom(replacement).Set(newVal)
		r.ParentMap.SetMapIndex(*r.ParentMapKey, replacement)
		return
	}

	// This should never happen since NewValueReflect ensures that the root object reflected on is a pointer and map
	// item replacement is handled above.
	panic(fmt.Sprintf("key %s may not be modified on struct: %T: struct is not settable", key, r.Value.Interface()))
}

func (r structReflect) Iterate(fn func(string, Value) bool) bool {
	vp := newTempValuePooler()
	defer vp.Recycle()
	return eachStructField(r.Value, func(s string, value reflect.Value) bool {
		return fn(s, vp.NewValueReflect(value))
	})
}


func (r structReflect) Range() MapRange {
	cacheEntry := TypeReflectEntryOf(r.Value.Type())
	fieldList := cacheEntry.FieldList()
	return &structReflectRange{r.Value, cacheEntry, fieldList, -1, len(fieldList), newTempValuePooler()}
}

type structReflectRange struct {
	val    reflect.Value
	cacheEntry TypeReflectCacheEntry
	fields []*FieldCacheEntry
	i int
	length int
	pooler *tempValuePooler
}

func (r *structReflectRange) Next() bool {
	r.i++
	for r.i < r.length {
		fieldEntry := r.fields[r.i]
		fieldVal := fieldEntry.GetFrom(r.val)
		omit := fieldEntry.isOmitEmpty && (safeIsNil(fieldVal) || isZero(fieldVal))
		if !omit {
			return true
		}
		r.i++
	}
	return false
}

func (r *structReflectRange) Key() string {
	if r.i < 0 {
		panic("Key() called before first calling Next()")
	}
	if r.i >= r.length {
		panic("Key() called on MapRange with no more items")
	}
	return r.fields[r.i].jsonName
}

func (r *structReflectRange) Value() Value {
	if r.i < 0 {
		panic("Value() called before first calling Next()")
	}
	if r.i >= r.length {
		panic("Value() called on MapRange with no more items")
	}
	return r.pooler.NewValueReflect(r.fields[r.i].GetFrom(r.val))
}

func (r *structReflectRange) Recycle() {
	r.pooler.Recycle()
}

func eachStructField(structVal reflect.Value, fn func(string, reflect.Value) bool) bool {
	for jsonName, fieldCacheEntry := range TypeReflectEntryOf(structVal.Type()).Fields() {
		fieldVal := fieldCacheEntry.GetFrom(structVal)
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
	structCacheEntry := TypeReflectEntryOf(r.Value.Type()).Fields()

	vp := newTempValuePooler()
	defer vp.Recycle()
	iter := m.Range()
	defer iter.Recycle()
	for iter.Next() {
		s := iter.Key()
		value := iter.Value()
		fieldCacheEntry, ok := structCacheEntry[s]
		if !ok {
			return false
		}
		lhsVal := fieldCacheEntry.GetFrom(r.Value)
		if !Equals(vp.NewValueReflect(lhsVal), value) {
			return false
		}
	}
	return true
}

func (r structReflect) findJsonNameFieldAndNotEmpty(jsonName string) (reflect.Value, bool) {
	structCacheEntry, ok := TypeReflectEntryOf(r.Value.Type()).Fields()[jsonName]
	if !ok {
		return reflect.Value{}, false
	}
	fieldVal := structCacheEntry.GetFrom(r.Value)
	omit := structCacheEntry.isOmitEmpty && (safeIsNil(fieldVal) || isZero(fieldVal))
	return fieldVal, !omit
}

func (r structReflect) findJsonNameField(jsonName string) (val reflect.Value, ok bool, omitEmpty bool) {
	structCacheEntry, ok := TypeReflectEntryOf(r.Value.Type()).Fields()[jsonName]
	if !ok {
		return reflect.Value{}, false, false
	}
	fieldVal := structCacheEntry.GetFrom(r.Value)
	return fieldVal, true, structCacheEntry.isOmitEmpty
}
