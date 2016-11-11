/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package main

import (
	"encoding/binary"
)

// TxHeader represents the transaction header format for obx. The header
// objects are tagged with multiple timestamps for latency measurements. The
// transaction blobs also contain other arbitrary payload data. The timestamps
// in the headers are time.Duration (ns) relative to the common start time.
type TxHeader struct {
	Tbroadcast uint64
	Tack       uint64
	Tarrived   uint64
	Tordered   uint64
	Treturned  uint64
	Tdelivered uint64
	Sequence   uint32 // Sequence # for this Server/Channel/Client
	Server     uint16 // Broadcast server #
	Channel    uint16 // Broadcast channel # (Redundant?)
	Client     uint16 // Server/Channel client #
}

const TxHeaderSize = 58 // bytes

// Put serializes a TxHeader into a byte buffer.
func (t *TxHeader) Put(buf []byte) {
	binary.BigEndian.PutUint64(buf[0:], t.Tbroadcast)
	binary.BigEndian.PutUint64(buf[8:], t.Tack)
	binary.BigEndian.PutUint64(buf[16:], t.Tarrived)
	binary.BigEndian.PutUint64(buf[24:], t.Tordered)
	binary.BigEndian.PutUint64(buf[32:], t.Treturned)
	binary.BigEndian.PutUint64(buf[40:], t.Tdelivered)
	binary.BigEndian.PutUint32(buf[48:], t.Sequence)
	binary.BigEndian.PutUint16(buf[52:], t.Server)
	binary.BigEndian.PutUint16(buf[54:], t.Channel)
	binary.BigEndian.PutUint16(buf[56:], t.Client)
}

// Get deserializes a TxHeader from a byte buffer.
func (t *TxHeader) Get(buf []byte) {
	t.Tbroadcast = binary.BigEndian.Uint64(buf[0:])
	t.Tack = binary.BigEndian.Uint64(buf[8:])
	t.Tarrived = binary.BigEndian.Uint64(buf[16:])
	t.Tordered = binary.BigEndian.Uint64(buf[24:])
	t.Treturned = binary.BigEndian.Uint64(buf[32:])
	t.Tdelivered = binary.BigEndian.Uint64(buf[40:])
	t.Sequence = binary.BigEndian.Uint32(buf[48:])
	t.Server = binary.BigEndian.Uint16(buf[52:])
	t.Channel = binary.BigEndian.Uint16(buf[54:])
	t.Client = binary.BigEndian.Uint16(buf[56:])
}
