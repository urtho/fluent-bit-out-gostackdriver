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

// FLBPluginRegister is fired upon plugin initialization
//export FLBPluginRegister 
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "gostackdriver", "Starckdriver output plugin.")
	//TODO: fetch GCP/GKE metadate
}

// FLBPluginInit is fired for every [OUTPUT] instance with plugin config handle
//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	id := output.FLBPluginConfigKey(plugin, "id")
	log.Printf("[gostackdriver] id = %q", id)
	// Set the context to point to any Go variable
	output.FLBPluginSetContext(plugin, id)

 	return output.FLB_OK
}

// FLBPluginFlush is called only for uninitialized instances
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


// FLBPluginFlushCtx is called for a set of buffered entiries from the same INPUT
// Can contain more entries then Stackdriver can support in a single batch
//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {

	//defer track(runningtime("Flush"))
	// Type assert context back into the original type for the Go variable
	//id := output.FLBPluginGetContext(ctx).(string)
	//log.Printf("[gostackdriver] Flush called for id: %s", id)

	dec := NewDecoder(data, int(length))
	//var json = jsoniter.ConfigCompatibleWithStandardLibrary

	count := 0
	for {
		rec := GetRecord(dec)
		if rec == nil {
			break
		}

		// Print record keys and values
		fmt.Printf("[%03d] Tag:%s TS:%s", count, C.GoString(tag), rec.ts.String())
		
		sdent := newEntry(rec)

		j, err := json.Marshal(sdent)
		if err != nil {
			fmt.Println("Cannot marshal JSON:", err)
		}
		fmt.Printf(" %s\n", j)

		//TODO - batch by 1000 entries max!
		count++
	}
	fmt.Printf("[gostackdriver] Entries: %d\n", count)

	//TODO - Do SYNC logging , return FLB_RETRY if error
	//This plugin should not ack uncommited entries
	//An error on batch no >#1 will result in duplicate entries when FLB retries :(
	return output.FLB_OK
}

// FLBPluginExit is called for plugin teardown
//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
