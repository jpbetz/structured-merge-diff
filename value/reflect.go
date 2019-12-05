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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var reflectPool = sync.Pool{
	New: func() interface{} {
		return &reflectValue{}
	},
}

var marshalerType = reflect.TypeOf(new(json.Marshaler)).Elem()

func Reflect(value interface{}) (Value, error) {
	return wrap(reflect.ValueOf(value))
}

func MustReflect(value interface{}) Value {
	v, err := Reflect(value)
	if err != nil {
		panic(err)
	}
	return v
}

func wrap(value reflect.Value) (Value, error) {
	if isCustomConvertable(value) {
		return toUnstructured(value)
	}
	rv := reflectPool.Get().(*reflectValue)
	rv.Value = value
	return Value(rv), nil
}

func mustWrap(value reflect.Value) Value {
	v, err := wrap(value)
	if err != nil {
		panic(err)
	}
	return v
}

func isCustomConvertable(rv reflect.Value) bool{
	// TODO: consider corner cases (typerefs with unmarshaller interface, etc...)
	switch rv.Kind() {
	case reflect.Ptr:
		return rv.Type().Implements(marshalerType)
	default:
		return reflect.PtrTo(rv.Type()).Implements(marshalerType)
	}
}

func toUnstructured(rv reflect.Value) (Value, error) {
	// TODO: round tripping through unstructured is expensive, can we avoid for both custom conversion and merging structured with unstructured?
	data, err := json.Marshal(rv.Interface())
	if err != nil {
		return nil, fmt.Errorf("error encoding %v to json: %v", rv, err)
	}
	wrappedResult := struct{Value interface{}}{}
	wrappedData := fmt.Sprintf("{\"Value\": %s}", data)
	err = json.Unmarshal([]byte(wrappedData), &wrappedResult)
	if err != nil {
		return nil, fmt.Errorf("error decoding %v from json: %v", data, err)
	}
	return  NewValueInterface(wrappedResult.Value), nil
}

type reflectValue struct {
	Value reflect.Value
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
	return safeIsNil(r.Value)
}
// TODO find a cleaner way to avoid panics from reflect.IsNil()
func safeIsNil(v reflect.Value) bool {
	k := v.Kind()
	switch k {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (r reflectValue) Map() Map {
	rval := deref(r.Value)
	switch rval.Kind() {
	case reflect.Struct:
		return reflectStruct{Value: r.Value}
	case reflect.Map:
		return reflectMap{Value: r.Value}
	default:
		panic("value is not a map or struct")
	}
}

func (r *reflectValue) Recycle() {
	reflectPool.Put(r)
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
	// TODO: What about reflect.Value.Uint?
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
	// In order to be mergable with unstructured, must return unstructured here
	v, err := toUnstructured(deref(r.Value))
	if err != nil {
		panic("unable to convert to unstructured via json round-trip")
	}
	return v.Interface()
}

type reflectMap struct {
	Value reflect.Value
}

func (r reflectMap) Length() int {
	rval := deref(r.Value)
	return rval.Len()
}

func (r reflectMap) Get(key string) (Value, bool) {
	var val reflect.Value
	rval := deref(r.Value)
	// TODO: this only works for strings and string alias key types
	val = rval.MapIndex(r.toMapKey(key))
	if !val.IsValid() {
		return nil, false
	}
	return mustWrap(val), val != zero
}

func (r reflectMap) Set(key string, val Value) {
	rval := deref(r.Value)
	rval.SetMapIndex(r.toMapKey(key), rval)
}

func (r reflectMap) Delete(key string) {
	rval := deref(r.Value)
	rval.SetMapIndex(r.toMapKey(key), zero)
}

func (r reflectMap) toMapKey(key string) reflect.Value {
	rval := deref(r.Value)
	return reflect.ValueOf(key).Convert(rval.Type().Key())
}

func (r reflectMap) Iterate(fn func(string, Value) bool) bool {
	rval := deref(r.Value)
	iter := rval.MapRange()
	for iter.Next() {
		next := iter.Value()
		if !next.IsValid() {
			continue
		}
		mapVal := mustWrap(next)
		if !fn(iter.Key().String(), mapVal) {
			mapVal.Recycle()
			return false
		}
		mapVal.Recycle()
	}
	return true
}

func (r reflectMap) Equals(m Map) bool {
	// TODO use reflect.DeepEqual
	return MapCompare(r, m) == 0
}

type reflectStruct struct {
	Value reflect.Value
	// TODO: is creating this lookup table worth the allocation?
	sync.Once
	fieldByJsonName map[string]reflect.Value
}

func (r reflectStruct) findJsonNameField(jsonName string) (reflect.Value, bool) {
	rval := deref(r.Value)
	r.Once.Do(func() {
		r.fieldByJsonName = make(map[string]reflect.Value, rval.NumField())
		r.accumulateFields(r.fieldByJsonName, rval)
	})
	field, ok := r.fieldByJsonName[jsonName]
	return field, ok
}

func (r reflectStruct) accumulateFields(fields map[string]reflect.Value, rval reflect.Value) {
	t := rval.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := rval.FieldByIndex(field.Index)
		if isInline(field) {
			r.accumulateFields(fields, rval.FieldByIndex(field.Index))
		} else if isOmitempty(field) && (safeIsNil(fieldVal) || isEmptyValue(fieldVal)) {
			// skip it
		} else {
			r.fieldByJsonName[lookupJsonName(field)] = fieldVal
		}
	}
}

func (r reflectStruct) Length() int {
	i := 0
	r.Iterate(func(s string, value Value) bool {
		i++
		return true
	})
	return i
}

func (r reflectStruct) Get(key string) (Value, bool) {
	if val, ok := r.findJsonNameField(key); ok {
		return mustWrap(val), true
	}
	// TODO: decide how to handle invalid keys
	return MustReflect(nil), false
}

func (r reflectStruct) Set(key string, val Value) {
	if val, ok := r.findJsonNameField(key); ok {
		val.Set(val)
	}
	// TODO: decide how to handle invalid keys
}

func (r reflectStruct) Delete(key string) {
	if val, ok := r.findJsonNameField(key); ok {
		val.Set(reflect.Value{})
	}
	// TODO: decide how to handle invalid keys
}

func (r reflectStruct) Iterate(fn func(string, Value) bool) bool {
	return r.iterate(deref(r.Value), fn)
}

func (r reflectStruct) iterate(rval reflect.Value, fn func(string, Value) bool) bool {
	for i := 0; i < rval.NumField(); i++ {
		field := rval.Type().Field(i)
		fieldVal := rval.FieldByIndex(field.Index)
		if isInline(field) {
			if ok := r.iterate(fieldVal, fn); !ok {
				return false
			}
		} else if isOmitempty(field) && (safeIsNil(fieldVal) || isEmptyValue(fieldVal)) {
			// skip it
		} else {
			fieldVal := mustWrap(rval.Field(i))
			if !fn(lookupJsonName(field), fieldVal) {
				fieldVal.Recycle()
				return false
			}
			fieldVal.Recycle()
		}
	}
	return true
}

func (r reflectStruct) Equals(m Map) bool {
	// TODO use reflect.DeepEqual
	return MapCompare(r, m) == 0
}

type ReflectList struct {
	Value reflect.Value
}

func (r ReflectList) Length() int {
	rval := deref(r.Value)
	return rval.Len()
}

func (r ReflectList) At(i int) Value {
	rval := deref(r.Value)
	return mustWrap(rval.Index(i))
}

var zero = reflect.Value{}

func isKind(val reflect.Value, kinds ...reflect.Kind) bool {
	rval := deref(val)
	kind := rval.Kind()
	for _, k := range kinds {
		if kind == k {
			return true
		}
	}
	return false
}

func deref(rval reflect.Value) reflect.Value {
	kind := rval.Kind()
	if kind == reflect.Interface || kind == reflect.Ptr {
		return rval.Elem()
	}
	return rval
}

func lookupJsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-"  {
		return f.Name
	}
	name, _ := parseTag(tag)
	if name == "" {
		return f.Name
	}
	return name
}

func isInline(f reflect.StructField) bool {
	return hasOpt(f, "inline")
}

func isOmitempty(f reflect.StructField) bool {
	return hasOpt(f, "omitempty")
}

func hasOpt(f reflect.StructField, opt string) bool {
	tag := f.Tag.Get("json")
	if tag == "-"  {
		return false
	}
	_, opts := parseTag(tag)
	return opts.Contains(opt)
}

// Copied from https://golang.org/src/encoding/json/encode.go

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

type tagOptions string

// parseTag splits a struct field's json tag into its name and
// comma-separated options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, tagOptions("")
}

// Contains reports whether a comma-separated list of options
// contains a particular substr flag. substr must be surrounded by a
// string boundary or commas.
func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == optionName {
			return true
		}
		s = next
	}
	return false
}