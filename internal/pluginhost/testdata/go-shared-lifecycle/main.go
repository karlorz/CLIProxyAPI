package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	backgroundActive atomic.Bool
	shutdownCalls    atomic.Int32
)

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
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	resp, _ := json.Marshal(map[string]any{
		"ok":             true,
		"shutdown_calls": shutdownCalls.Load(),
	})
	ptr := C.CBytes(resp)
	if response != nil {
		response.ptr = ptr
		response.len = C.size_t(len(resp))
	}
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	count := shutdownCalls.Add(1)
	backgroundActive.Store(false)
	if markerFile := os.Getenv("TEST_LIFECYCLE_MARKER_FILE"); markerFile != "" {
		_ = os.WriteFile(markerFile, []byte(fmt.Sprintf("%d", count)), 0o644)
	}
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}

	mode := os.Getenv("TEST_LIFECYCLE_INIT_MODE")
	switch mode {
	case "error":
		// Export shutdown table before returning 1 so init failure shutdown cleanup is exercised.
		plugin.abi_version = 1
		plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
		plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
		plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
		return 1
	case "wrong_abi":
		plugin.abi_version = 99999
		plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
		plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
		plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
		return 0
	case "incomplete_api":
		plugin.abi_version = 1
		plugin.call = nil
		plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
		plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
		return 0
	default:
		plugin.abi_version = 1
		plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
		plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
		plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
		return 0
	}
}
