package main

import (
	"unsafe"

	"C"
	"github.com/fluent/fluent-bit-go/output"
	_ "github.com/philips-software/go-hsdp-api/logging"
)

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "hsdp", "HSDP logging")
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	// do something with the data
	return 0
}

//export FLBPluginExit
func FLBPluginExit() int {
	return 0
}

func main() {
}
