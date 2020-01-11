package main

import (
//	"bytes"
//	"context"
//	"encoding/json"
//	"errors"
//	"fmt"
//	"log"
//	"net/http"
//	"os"
	"strings"
	"sync"

	"cloud.google.com/go/compute/metadata"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	sdlog "cloud.google.com/go/logging"
)

var enumSeverityMap = map[string]logtypepb.LogSeverity{
	"EMERGENCY": logtypepb.LogSeverity_EMERGENCY,
	"EMERG":     logtypepb.LogSeverity_EMERGENCY,
	"A":         logtypepb.LogSeverity_ALERT,
	"ALERT":     logtypepb.LogSeverity_ALERT,
	"C":         logtypepb.LogSeverity_CRITICAL,
	"F":         logtypepb.LogSeverity_CRITICAL,
	"CRIT":      logtypepb.LogSeverity_CRITICAL,
	"FATAL":     logtypepb.LogSeverity_CRITICAL,
	"CRITICAL":  logtypepb.LogSeverity_CRITICAL,
	"E":         logtypepb.LogSeverity_ERROR,
	"ERR":       logtypepb.LogSeverity_ERROR,
	"ERROR":     logtypepb.LogSeverity_ERROR,
	"SEVERE":    logtypepb.LogSeverity_ERROR,
	"W":         logtypepb.LogSeverity_WARNING,
	"WARN":      logtypepb.LogSeverity_WARNING,
	"WARNING":   logtypepb.LogSeverity_WARNING,
	"N":         logtypepb.LogSeverity_NOTICE,
	"NOTICE":    logtypepb.LogSeverity_NOTICE,
	"I":         logtypepb.LogSeverity_INFO,
	"INFO":      logtypepb.LogSeverity_INFO,
	"D":         logtypepb.LogSeverity_DEBUG,
	"DEBUG":     logtypepb.LogSeverity_DEBUG,
	"TRACE":     logtypepb.LogSeverity_DEBUG,
	"TRACE_INT": logtypepb.LogSeverity_DEBUG,
	"FINE":      logtypepb.LogSeverity_DEBUG,
	"FINER":     logtypepb.LogSeverity_DEBUG,
	"FINEST":    logtypepb.LogSeverity_DEBUG,
	"CONFIG":    logtypepb.LogSeverity_DEBUG,
	"DEFAULT":   logtypepb.LogSeverity_DEFAULT,
}

//mapSeverity Map severities in the wild to an integer value 
func mapSeverity(sev string) logtypepb.LogSeverity {
	if sl, e := enumSeverityMap[strings.ToUpper(sev)]; e {
		return sl
	}
	return logtypepb.LogSeverity_DEFAULT
}

var detectedResource struct {
	pb   *mrpb.MonitoredResource
	once sync.Once
}

func detectGCEResource() *mrpb.MonitoredResource {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil
	}
	id, err := metadata.InstanceID()
	if err != nil {
		return nil
	}
	zone, err := metadata.Zone()
	if err != nil {
		return nil
	}
	name, err := metadata.InstanceName()
	if err != nil {
		return nil
	}
	return &mrpb.MonitoredResource{
		Type: "gce_instance",
		Labels: map[string]string{
			"project_id":    projectID,
			"instance_id":   id,
			"instance_name": name,
			"zone":          zone,
		},
	}
}

func detectResource() *mrpb.MonitoredResource {
	detectedResource.once.Do(func() {
		detectedResource.pb = detectGCEResource()
	})
	return detectedResource.pb
}

func newEntry(rec *FLBRecord) *sdlog.Entry {
	var k8smap map[string]interface{} 
	if k8s, e := rec.kv["kubernetes"]; e {
		if k8sm, ok := k8s.(map[string]interface{}); ok {
			k8smap = k8sm
			delete(rec.kv,"kubernetes")
		}
	}
	rec.cleanUp()
	return &sdlog.Entry {
		Timestamp : rec.ts.Time,
		Severity : rec.parseSeverity(k8smap),
		Trace: rec.popTrace(),
		SpanID: rec.popSpanID(),
		Labels: rec.popLabels(k8smap),
		Resource: rec.popResource(k8smap),
		LogName: rec.popLogName(),
		Payload: rec.kv,
	}
}

func (r *FLBRecord) popTrace() string {
	//FIXME
	return ""
}
func (r *FLBRecord) popSpanID() string {
	//FIXME
	return ""
}
func (r *FLBRecord) popLogName() string {
	//FIXME
	return ""
}

//popLabels unnests kubernetes.labels and prepends all keys with "k8s-pod/"
func (r *FLBRecord) popLabels(k8sm map[string]interface{}) map[string]string {
	lbls := make(map[string]string)
	if k8sm == nil {
		return lbls
	}
	if k8slbls, e := k8sm["labels"]; e {
		if k8slblsm, ok := k8slbls.(map[string]interface{}); ok {
			for k,v := range k8slblsm {
				if vs, ok := v.(string); ok {
					lbls["k8s-pod/"+k] = vs
				}
			}	
		}
		delete(k8sm,"labels")
	}
	return lbls
}

//popResouce fills resource labels with detected metadata overwritten by fields from kubernetes 
func (r *FLBRecord) popResource(k8sm map[string]interface{}) *mrpb.MonitoredResource {
	dr := detectResource()
	if (k8sm == nil) {
		return dr
	}
	var ndr mrpb.MonitoredResource
	ndr.Type = "k8s_container"
	ndr.Labels = make(map[string]string)
	for k,v := range dr.Labels {
		ndr.Labels[k] = v
	}	
	for k,v := range k8sm {
		if vs, ok := v.(string); ok {
			ndr.Labels[k] = vs
		}
	}
	return &ndr	
}

//cleanUp deletes redudant information from payload
func (r *FLBRecord) cleanUp() {
	delete(r.kv,"log")
	//FIXME
//	delete(r.kv,"timestamp")
//	delete(r.kv,"time")
//	delete(r.kv,"stream")
}

func (r *FLBRecord) parseSeverity(k8sm map[string]interface{}) sdlog.Severity {
	if sev,e := k8sm["severity"]; e {
		if ssev, ok := sev.(string); ok {
			return sdlog.Severity(mapSeverity(ssev))
		}		
	}

	if sev,e := r.kv["severity"]; e {
		if ssev, ok := sev.(string); ok {
			return sdlog.Severity (mapSeverity(ssev))
		}
	}
	delete(r.kv,"severity")
	return sdlog.Severity (logtypepb.LogSeverity_DEFAULT)
}

func init() {
	detectResource()
}