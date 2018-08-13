package main

import (
	"C"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/m4rw3r/uuid"
	"github.com/philips-software/go-hsdp-api/logging"
)

var (
	// both variables are set in Makefile
	revision  string
	builddate string
	plugin    Plugin = &fluentPlugin{}
	client    *logging.Client
)

type fluentPlugin struct{}

type Plugin interface {
	Environment(ctx unsafe.Pointer, key string) string
	Unregister(ctx unsafe.Pointer)
	GetRecord(dec *output.FLBDecoder) (ret int, ts interface{}, rec map[interface{}]interface{})
	NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder
	Send(values []logging.Resource) error
	Exit(code int)
}

func (p *fluentPlugin) Environment(ctx unsafe.Pointer, key string) string {
	// Environment variables have priority
	envKey := "HSDP_" + key
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return output.FLBPluginConfigKey(ctx, key)
}

func (p *fluentPlugin) Unregister(ctx unsafe.Pointer) {
	output.FLBPluginUnregister(ctx)
}

func (p *fluentPlugin) GetRecord(dec *output.FLBDecoder) (int, interface{}, map[interface{}]interface{}) {
	return output.GetRecord(dec)
}

func (p *fluentPlugin) NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder {
	return output.NewDecoder(data, int(length))
}

func (p *fluentPlugin) Exit(code int) {
	os.Exit(code)
}

func (p *fluentPlugin) Send(values []logging.Resource) error {
	// TODO
	return nil
}

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "hsdp", "HSDP logging output plugin")
}

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	host := plugin.Environment(ctx, "Host")
	sharedKey := plugin.Environment(ctx, "SharedKey")
	secretKey := plugin.Environment(ctx, "SecretKey")
	productKey := plugin.Environment(ctx, "ProductKey")
	var err error

	client, err = logging.NewClient(nil,
		logging.Config{
			SharedKey:    sharedKey,
			SharedSecret: secretKey,
			ProductKey:   productKey,
			BaseURL:      host,
		})
	if err != nil {
		fmt.Printf("configuration errors: %v\n", err)
		plugin.Unregister(ctx)
		plugin.Exit(1)
		return output.FLB_ERROR
	}
	fmt.Printf("[out-hsdp] build:%s version:%s\n", builddate, revision)
	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	// do something with the data
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}

	var resources []logging.Resource

	// Create Fluent Bit decoder
	dec := plugin.NewDecoder(data, int(length))

	// TODO trigger store at 25 records
	for {
		// Extract Record
		ret, ts, record = plugin.GetRecord(dec)
		if ret != 0 {
			break
		}

		// Print record keys and values
		var timeStamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timeStamp = ts.(output.FLBTime).Time
		case uint64:
			timeStamp = time.Unix(int64(t), 0)
		default:
			fmt.Print("given time is not in a known format, defaulting to now.\n")
			timeStamp = time.Now()
		}

		js, err := createResource(timeStamp, C.GoString(tag), record)
		if err != nil {
			fmt.Printf("%v\n", err)
			// DO NOT RETURN HERE becase one message has an error when json is
			// generated, but a retry would fetch ALL messages again. instead an
			// error should be printed to console
			continue
		}
		resources = append(resources, *js)
	}
	fmt.Printf("[out-hsdp] flushing %d resources\n", len(resources))

	_, err := client.StoreResources(resources, len(resources))
	// Iterate Records

	// Return options:
	//
	// output.FLB_OK    = data have been processed.
	// output.FLB_ERROR = unrecoverable error, do not try this again.
	// output.FLB_RETRY = retry to flush later.
	if err != nil {
		fmt.Printf("[out-hsdp] error: %v\n", err)
		return output.FLB_ERROR
	}
	return output.FLB_OK
}

func createResource(timestamp time.Time, tag string, record map[interface{}]interface{}) (*logging.Resource, error) {
	m := make(map[string]interface{})
	// convert timestamp to RFC3339Nano which is logstash format
	m["@timestamp"] = timestamp.UTC().Format(time.RFC3339Nano)
	m["@tag"] = tag
	for k, v := range record {
		switch t := v.(type) {
		case []byte:
			// prevent encoding to base64
			m[k.(string)] = string(t)
		default:
			m[k.(string)] = v
		}
	}
	msg, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error creating message for hsdp-logging: %v", err)
	}
	id, _ := uuid.V4()
	transactionID, _ := uuid.V4()
	serverName := "fluent-bit"
	if sn := m["host"].(string); sn != "" {
		serverName = sn

	}

	return &logging.Resource{
		ID:                  id.String(),
		Severity:            "INFO",
		ApplicationInstance: "fluent-bit",
		ApplicationName:     "fluent-bit",
		OriginatingUser:     "fluent-bit",
		Category:            "TRACELOG",
		Component:           "fluent-bit",
		ApplicationVersion:  "1.0",
		ServerName:          serverName,
		ServiceName:         "fluent-bit",
		EventID:             "1",
		TransactionID:       transactionID.String(),
		LogTime:             timestamp.UTC().Format(logging.LogTimeFormat),
		LogData:             logging.LogData{Message: string(msg)},
	}, nil
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
