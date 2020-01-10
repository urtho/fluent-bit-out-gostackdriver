package main

import (
	"C"
	"fmt"
	"log"
	"unsafe"
	"time"	
	json "github.com/json-iterator/go"
	"github.com/fluent/fluent-bit-go/output"
)

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "gostackdriver", "Testing multiple instances.")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	id := output.FLBPluginConfigKey(plugin, "id")
	log.Printf("[gostackdriver] id = %q", id)
	// Set the context to point to any Go variable
	output.FLBPluginSetContext(plugin, id)

 	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	log.Print("[gostackdriver] Flush called for unknown instance")
	return output.FLB_OK
}

func runningtime(s string) (string, time.Time) {
    return s, time.Now()
}

func track(s string, startTime time.Time) {
    endTime := time.Now()
    log.Println("End:	", s, "took", endTime.Sub(startTime))
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {

	defer track(runningtime("Flush"))
	// Type assert context back into the original type for the Go variable
	//id := output.FLBPluginGetContext(ctx).(string)
	//log.Printf("[gostackdriver] Flush called for id: %s", id)

	dec := NewDecoder(data, int(length))

	count := 0
	for {
		rec := GetRecord(dec)
		if rec == nil {
			break
		}

		// Print record keys and values
		//fmt.Printf("[%03d] Tag:%s TS:%s", count, C.GoString(tag), rec.ts.String())

		_, err := json.Marshal(rec.kv)
		if err != nil {
			fmt.Println("Cannot marshal JSON:", err)
		}
		//fmt.Printf(" %s\n", j)

		count++
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
