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

const (
	Broadcast = "Broadcast"
	Deliver   = "Deliver"
)

// Client represents a broadcast or deliver client for RPC callbacks.
type Client struct {
	Type    string // Broadcast or Deliver
	Server  int
	Channel int
	Client  int
}

// ClientData represents a broadcast or deliver client for RPC callbacks that
// require data.
type ClientData struct {
	Client
	Data interface{}
}
