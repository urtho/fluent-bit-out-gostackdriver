//  Fluent Bit Go!
//  ==============
//  Copyright (C) 2015-2017 Treasure Data Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package main

import (
	"unsafe"
	"reflect"
	"encoding/binary"
	"github.com/ugorji/go/codec"
	"time"
	"C"
)

type FLBDecoder struct {
	handle *codec.MsgpackHandle
	mpdec  *codec.Decoder
}

type FLBTime struct {
	time.Time
}

type FLBRecord struct {
	ts FLBTime
	kv map[string]interface{}
}

func (f FLBTime) WriteExt(interface{}) []byte {
	panic("unsupported")
}

func (f FLBTime) ReadExt(i interface{}, b []byte) {
	out := i.(*FLBTime)
	sec := binary.BigEndian.Uint32(b)
	usec := binary.BigEndian.Uint32(b[4:])
	out.Time = time.Unix(int64(sec), int64(usec))
}

func (f FLBTime) ConvertExt(v interface{}) interface{} {
	return nil
}

func (f FLBTime) UpdateExt(dest interface{}, v interface{}) {
	panic("unsupported")
}

func NewDecoder(data unsafe.Pointer, length int) (*FLBDecoder) {
	var b []byte

	dec := new(FLBDecoder)
	dec.handle = new(codec.MsgpackHandle)
	dec.handle.MapType = reflect.TypeOf(map[string]interface{}(nil))
	dec.handle.SetExt(reflect.TypeOf(FLBTime{}), 0, &FLBTime{})

	b = C.GoBytes(data, C.int(length))
	dec.mpdec = codec.NewDecoderBytes(b, dec.handle)

	return dec
}


func GetRecord(dec *FLBDecoder) *FLBRecord {
	var check error
	var r FLBRecord
	var ra []interface{}

	check = dec.mpdec.Decode(&ra)
	if check != nil {
		return nil
	}

	r.ts = ra[0].(FLBTime)
	r.kv = ra[1].(map[string]interface{})

	return &r
}
