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
	"reflect"
)

type mapUnstructuredInterface map[interface{}]interface{}

func (m mapUnstructuredInterface) Set(key string, val Value) {
	m[key] = val.Unstructured()
}

func (m mapUnstructuredInterface) Get(key string) (Value, bool) {
	if v, ok := m[key]; !ok {
		return nil, false
	} else {
		return NewValueInterface(v), true
	}
}

func (m mapUnstructuredInterface) Has(key string) bool {
	_, ok := m[key]
	return ok
}

func (m mapUnstructuredInterface) Delete(key string) {
	delete(m, key)
}

func (m mapUnstructuredInterface) Iterate(fn func(key string, value Value) bool) bool {
	if len(m) == 0 {
		return true
	}
	vv := viPool.Get().(*valueUnstructured)
	defer vv.Recycle()
	for k, v := range m {
		if ks, ok := k.(string); !ok {
			continue
		} else {
			vv.Value = v
			if !fn(ks, vv) {
				return false
			}
		}
	}
	return true
}


func (m mapUnstructuredInterface) Range() MapRange {
	vv := viPool.Get().(*valueUnstructured)
	return &mapUnstructuredRange{reflect.ValueOf(m).MapRange(), vv}
}

func (m mapUnstructuredInterface) Length() int {
	return len(m)
}

func (m mapUnstructuredInterface) Equals(other Map) bool {
	if m.Length() != other.Length() {
		return false
	}
	if len(m) == 0 {
		return true
	}
	vv := viPool.Get().(*valueUnstructured)
	defer vv.Recycle()
	for k, v := range m {
		ks, ok := k.(string)
		if !ok {
			return false
		}
		vo, ok := other.Get(ks)
		if !ok {
			return false
		}
		vv.Value = v
		if !Equals(vv, vo) {
			vo.Recycle()
			return false
		}
		vo.Recycle()
	}
	return true
}

type mapUnstructuredString map[string]interface{}

func (m mapUnstructuredString) Set(key string, val Value) {
	m[key] = val.Unstructured()
}

func (m mapUnstructuredString) Get(key string) (Value, bool) {
	if v, ok := m[key]; !ok {
		return nil, false
	} else {
		return NewValueInterface(v), true
	}
}

func (m mapUnstructuredString) Has(key string) bool {
	_, ok := m[key]
	return ok
}

func (m mapUnstructuredString) Delete(key string) {
	delete(m, key)
}

func (m mapUnstructuredString) Iterate(fn func(key string, value Value) bool) bool {
	if len(m) == 0 {
		return true
	}
	vv := viPool.Get().(*valueUnstructured)
	defer vv.Recycle()
	for k, v := range m {
		vv.Value = v
		if !fn(k, vv) {
			return false
		}
	}
	return true
}


func (m mapUnstructuredString) Range() MapRange {
	vv := viPool.Get().(*valueUnstructured)
	return &mapUnstructuredRange{reflect.ValueOf(m).MapRange(), vv}
}

type mapUnstructuredRange struct {
	iter    *reflect.MapIter
	vv     *valueUnstructured
}

func (r *mapUnstructuredRange) Next() bool {
	return r.iter.Next()
}

func (r *mapUnstructuredRange) Key() string {
	// TODO(jpbetz): Avoid this cast
	return r.iter.Key().Interface().(string)
}

func (r *mapUnstructuredRange) Value() Value {
	r.vv.Value = r.iter.Value().Interface()
	return r.vv
}

func (r *mapUnstructuredRange) Recycle() {
	r.vv.Recycle()
}

func (m mapUnstructuredString) Length() int {
	return len(m)
}

func (m mapUnstructuredString) Equals(other Map) bool {
	if m.Length() != other.Length() {
		return false
	}
	if len(m) == 0 {
		return true
	}
	vv := viPool.Get().(*valueUnstructured)
	defer vv.Recycle()
	for k, v := range m {
		vo, ok := other.Get(k)
		if !ok {
			return false
		}
		vv.Value = v
		if !Equals(vv, vo) {
			vo.Recycle()
			return false
		}
		vo.Recycle()
	}
	return true
}
