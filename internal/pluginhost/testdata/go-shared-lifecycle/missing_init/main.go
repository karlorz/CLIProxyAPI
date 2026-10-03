package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;
*/
import "C"

import (
	"sync/atomic"
	"time"
	"unsafe"
)

var backgroundActive atomic.Bool

func init() {
	backgroundActive.Store(true)
	go func() {
		for backgroundActive.Load() {
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func main() {}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {}
