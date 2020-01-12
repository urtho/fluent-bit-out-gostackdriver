package main

import (
	"cloud.google.com/go/compute/metadata"
	vkit "cloud.google.com/go/logging/apiv2"
	"context"
	"errors"
	"fmt"
	"log"
	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	json "github.com/json-iterator/go"
	"google.golang.org/api/option"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
	"strings"
	"sync"
	"time"
)

const (
	// ProdAddr is the production address.
	ProdAddr = "logging.googleapis.com:443"
	// WriteScope - use WriteScope only
	WriteScope = "https://www.googleapis.com/auth/logging.write"
	// EntriesMax flush up to 1000 entries at a time
	EntriesMax = 1000
)

type sdClient struct {
	client   *vkit.Client // client for the logging service
	closed   bool
	entries  []*logpb.LogEntry
	logName  string
	resource *mrpb.MonitoredResource
	labels   map[string]string
}

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
		log.Printf("Error getting projectID : %s\n", err)
		return nil
	}
	id, err := metadata.InstanceID()
	if err != nil {
		log.Printf("Error getting instanceID : %s\n", err)
	}
	zone, err := metadata.Zone()
	if err != nil {
		log.Printf("Error getting zone : %s\n", err)
	}
	name, err := metadata.InstanceName()
	if err != nil {
		log.Printf("Error getting instanceName : %s\n", err)
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

// toProtoStruct converts v, which must marshal into a JSON object,
// into a Google Struct proto.
func toProtoStruct(v interface{}) (*structpb.Struct, error) {
	// Fast path: if v is already a *structpb.Struct, nothing to do.
	if s, ok := v.(*structpb.Struct); ok {
		return s, nil
	}
	// v is a Go value that supports JSON marshalling. We want a Struct
	// protobuf. Some day we may have a more direct way to get there, but right
	// now the only way is to marshal the Go value to JSON, unmarshal into a
	// map, and then build the Struct proto from the map.
	var jb []byte
	var err error
	if raw, ok := v.(json.RawMessage); ok { // needed for Go 1.7 and below
		jb = []byte(raw)
	} else {
		jb, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("logging: json.Marshal: %v", err)
		}
	}
	var m map[string]interface{}
	err = json.Unmarshal(jb, &m)
	if err != nil {
		return nil, fmt.Errorf("logging: json.Unmarshal: %v", err)
	}
	return jsonMapToProtoStruct(m), nil
}

func jsonMapToProtoStruct(m map[string]interface{}) *structpb.Struct {
	fields := map[string]*structpb.Value{}
	for k, v := range m {
		fields[k] = jsonValueToStructValue(v)
	}
	return &structpb.Struct{Fields: fields}
}

func jsonValueToStructValue(v interface{}) *structpb.Value {
	switch x := v.(type) {
	case bool:
		return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: x}}
	case float64:
		return &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: x}}
	case string:
		return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: x}}
	case nil:
		return &structpb.Value{Kind: &structpb.Value_NullValue{}}
	case map[string]interface{}:
		return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: jsonMapToProtoStruct(x)}}
	case []interface{}:
		var vals []*structpb.Value
		for _, e := range x {
			vals = append(vals, jsonValueToStructValue(e))
		}
		return &structpb.Value{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: vals}}}
	default:
		panic(fmt.Sprintf("bad type %T for JSON value", v))
	}
}

func (c *sdClient) appendEntry(rec *FLBRecord) error {
	var k8smap map[string]interface{}
	if k8s, e := rec.kv["kubernetes"]; e {
		if k8sm, ok := k8s.(map[string]interface{}); ok {
			k8smap = k8sm
			delete(rec.kv, "kubernetes")
		}
	}
	ts, err := ptypes.TimestampProto(rec.ts.Time)
	if err != nil {
		ts, err = ptypes.TimestampProto(time.Now())
		if err != nil {
			//The Time has ended
			return nil
		}
	}
	rec.cleanUp()

	//FIXME: This sucks ... up the CPU!
	s, err := toProtoStruct(rec.kv)
	if err != nil {
		return nil
	}
	if c.labels == nil {
		c.labels = rec.popLabels(k8smap)
	}

	if c.resource == nil {
		c.resource = rec.popResource(k8smap)
	}

	c.entries = append(c.entries, &logpb.LogEntry{
		Timestamp: ts,
		Severity:  rec.parseSeverity(k8smap),
		Trace:     rec.popTrace(),
		SpanId:    rec.popSpanID(),
		Payload:   &logpb.LogEntry_JsonPayload{JsonPayload: s},
	})
	if len(c.entries) >= EntriesMax {
		return c.flush()
	}
	return nil
}

func (c *sdClient) reset(tag string) error {
	c.entries = c.entries[:0]
	dr := detectResource()
	projectID, ok := dr.Labels["project_id"]
	if !ok {
		return errors.New("project_id not detected")
	}
	c.logName = fmt.Sprintf("projects/%s/logs/%s", projectID, tag)
	c.resource = nil
	c.labels = nil
	return nil
}

func (c *sdClient) flush() error {
	if len(c.entries) == 0 {
		return nil
	}
	_, err := c.client.WriteLogEntries(context.Background(), &logpb.WriteLogEntriesRequest{
		LogName:  c.logName,
		Resource: c.resource,
		Labels:   c.labels,
		Entries:  c.entries,
	})
	c.entries = c.entries[:0]
	return err
}

func (r *FLBRecord) popTrace() string {
	//FIXME
	return ""
}
func (r *FLBRecord) popSpanID() string {
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
			for k, v := range k8slblsm {
				if vs, ok := v.(string); ok {
					lbls["k8s-pod/"+k] = vs
				}
			}
		}
		delete(k8sm, "labels")
	}
	return lbls
}

//popResouce fills resource labels with detected metadata overwritten by fields from kubernetes
func (r *FLBRecord) popResource(k8sm map[string]interface{}) *mrpb.MonitoredResource {
	dr := detectResource()
	if k8sm == nil {
		return dr
	}
	var ndr mrpb.MonitoredResource
	ndr.Type = "k8s_container"
	ndr.Labels = make(map[string]string)
	for k, v := range dr.Labels {
		ndr.Labels[k] = v
	}
	for k, v := range k8sm {
		if vs, ok := v.(string); ok {
			ndr.Labels[k] = vs
		}
	}
	return &ndr
}

//cleanUp deletes redudant information from payload
func (r *FLBRecord) cleanUp() {
	delete(r.kv, "log")
	//FIXME
	//	delete(r.kv,"timestamp")
	delete(r.kv, "time")
	//	delete(r.kv,"stream")
}

func (r *FLBRecord) parseSeverity(k8sm map[string]interface{}) logtypepb.LogSeverity {
	if sev, e := k8sm["severity"]; e {
		if ssev, ok := sev.(string); ok {
			delete(k8sm, "severity")
			return mapSeverity(ssev)
		}
	}

	if sev, e := r.kv["severity"]; e {
		if ssev, ok := sev.(string); ok {
			delete(r.kv, "severity")
			return mapSeverity(ssev)
		}
	}
	return logtypepb.LogSeverity_DEFAULT
}

func init() {
	r := detectResource()
	log.Printf("[gostackdriver] GCPLabels: %v\n", r.Labels)
}

// close closes underlying client
func (c *sdClient) close() error {
	if c.closed {
		return nil
	}
	c.flush()
	err2 := c.client.Close()
	c.closed = true
	return err2
}

// newSDClient creates new sdClient
func newSDClient(ctx context.Context, opts ...option.ClientOption) (*sdClient, error) {
	opts = append([]option.ClientOption{
		option.WithEndpoint(ProdAddr),
		option.WithScopes(WriteScope),
	}, opts...)

	c, err := vkit.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	//FIXME: this one is "not legal"
	c.SetGoogleClientInfo("fbitgo", "1")
	return &sdClient{
		client:  c,
		closed:  false,
		entries: make([]*logpb.LogEntry, 0, EntriesMax),
	}, nil
}

func testSDClient() error {
	c, e := newSDClient(context.Background())
	if e != nil {
		return e
	}
	ts, err := ptypes.TimestampProto(time.Unix(0, 0))
	if err != nil {
		return err
	}
	r := detectResource()
	parent, ok := r.Labels["project_id"]
	if !ok {
		return errors.New("No project id detected")
	}

	c.entries = append(c.entries, &logpb.LogEntry{
		LogName:   fmt.Sprintf("projects/%s/logs/ping", parent),
		Resource:  r,
		Payload:   &logpb.LogEntry_TextPayload{TextPayload: "ping"},
		Timestamp: ts,     // Identical timestamps and insert IDs are both
		InsertId:  "ping", // necessary for the service to dedup these entries.
	})
	return c.flush()
}
