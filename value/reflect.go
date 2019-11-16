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

func Reflect(value interface{}) Value {
	return reflectValue{Value: value}
}

type reflectValue struct {
	Value interface{}
}

func (r reflectValue) IsMap() bool {
	return isKind(r.Value, reflect.Map, reflect.Struct)
}

func (r reflectValue) IsList() bool {
	return isKind(r.Value, reflect.Slice, reflect.Array)
}
func (r reflectValue) IsBool() bool {
	return isKind(r.Value, reflect.Bool)
}
func (r reflectValue) IsInt() bool {
	// This feels wrong. Very wrong.
	return isKind(r.Value, reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Uint64, reflect.Uint, reflect.Uint32, reflect.Uint16, reflect.Uint8)
}
func (r reflectValue) IsFloat() bool {
	return isKind(r.Value, reflect.Float64, reflect.Float32)
}
func (r reflectValue) IsString() bool {
	return isKind(r.Value, reflect.String)
}
func (r reflectValue) IsNull() bool {
	return reflect.ValueOf(r.Value).IsNil()
}
func (r reflectValue) Map() Map {
	rval := deref(r.Value)
	switch rval.Kind() {
	case reflect.Struct:
		return reflectStruct{r.Value}
	case reflect.Map:
		return reflectMap{r.Value}
	default:
		panic("value is not a map or struct")
	}
}

func (r reflectValue) List() List {
	if r.IsList() {
		return ReflectList{r.Value}
	}
	panic("value is not a list")
}
func (r reflectValue) Bool() bool {
	if r.IsBool() {
		return deref(r.Value).Bool()
	}
	panic("value is not a bool")
}
func (r reflectValue) Int() int64 {
	if r.IsInt() {
		return deref(r.Value).Int()
	}
	panic("value is not an int")
}
func (r reflectValue) Float() float64 {
	if r.IsFloat() {
		return deref(r.Value).Float()
	}
	panic("value is not a float")
}
func (r reflectValue) String() string {
	if r.IsString() {
		return deref(r.Value).String()
	}
	panic("value is not a string")
}
func (r reflectValue) Interface() interface{} {
	return r.Value
}

type reflectMap struct {
	Value interface{}
}

func (r reflectMap) Length() int {
	rval := deref(r.Value)
	return rval.Len()
}

func (r reflectMap) Get(key string) (Value, bool) {
	var val reflect.Value
	rval := deref(r.Value)
	val = rval.MapIndex(reflect.ValueOf(key))
	return reflectValue{val.Interface()}, val != zero
}

func (r reflectMap) Set(key string, val Value) {
	rval := deref(r.Value)
	rval.SetMapIndex(reflect.ValueOf(key), rval)
}

func (r reflectMap) Delete(key string) {
	rval := deref(r.Value)
	rval.SetMapIndex(reflect.ValueOf(key), reflect.Value{})
}

func (r reflectMap) Iterate(fn func(string, Value) bool) {
	rval := deref(r.Value)
	iter := rval.MapRange()
	for iter.Next() {
		if !fn(iter.Key().String(), reflectValue{iter.Value().Interface()}) {
			return
		}
	}
}

type reflectStruct struct {
	Value interface{}
}

func (r reflectStruct) Length() int {
	rval := deref(r.Value)
	return rval.NumField()
}

func (r reflectStruct) Get(key string) (Value, bool) {
	var val reflect.Value
	rval := deref(r.Value)
	val = rval.FieldByName(key)
	return reflectValue{val.Interface()}, val != zero
}

func (r reflectStruct) Set(key string, val Value) {
	rval := deref(r.Value)
	rval.FieldByName(key).Set(rval)
}

func (r reflectStruct) Delete(key string) {
	rval := deref(r.Value)
	rval.FieldByName(key).Set(reflect.Value{})
}

func (r reflectStruct) Iterate(fn func(string, Value) bool) {
	rval := deref(r.Value)
	for i := 0; i < rval.NumField(); i++ {
		fn(rval.Type().Field(i).Name, reflectValue{rval.Field(i).Interface()})
	}
}

type ReflectList struct {
	Value interface{}
}

// TODO: This function should not be part of the value.List interface
func (r ReflectList) Interface() []interface{} {
	result := make([]interface{}, r.Length(), r.Length())
	r.Iterate(func(i int, value Value) {
		result[i] = value.Interface()
	})
	return result
}

func (r ReflectList) Length() int {
	rval := deref(r.Value)
	return rval.Len()
}

func (r ReflectList) Iterate(fn func(int, Value)) {
	rval := deref(r.Value)
	length := rval.Len()
	for i := 0; i < length; i++ {
		fn(i, reflectValue{rval.Index(i).Interface()})
	}
}

func (r ReflectList) At(i int) Value {
	rval := deref(r.Value)
	return reflectValue{rval.Index(i).Interface()}
}

var zero = reflect.Value{}

func isKind(val interface{}, kinds ...reflect.Kind) bool {
	rval := deref(val)
	kind := rval.Kind()
	for _, k := range kinds {
		if kind == k {
			return true
		}
	}
	return false
}

func deref(val interface{}) reflect.Value {
	rval := reflect.ValueOf(val)
	kind := rval.Type().Kind()
	if kind == reflect.Interface || kind == reflect.Ptr {
		return rval.Elem()
	}
	return rval
}