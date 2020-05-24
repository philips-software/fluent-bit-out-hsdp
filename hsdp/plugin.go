package hsdp

import (
	"C"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/google/uuid"
	"github.com/philips-software/go-hsdp-api/logging"
)

var (
	// both variables are set in Makefile
	revision  string
	builddate string
	plugin    Plugin = &fluentPlugin{}
	client    *logging.Client
	resources []logging.Resource
	useCustomField bool
)

const (
	batchSize = 25
)

type fluentPlugin struct{}

// Plugin interface for verifying all methods are available
type Plugin interface {
	Environment(ctx unsafe.Pointer, key string) string
	Unregister(ctx unsafe.Pointer)
	GetRecord(dec *output.FLBDecoder) (ret int, ts interface{}, rec map[interface{}]interface{})
	NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder
	Exit(code int)
}

func (p *fluentPlugin) Environment(ctx unsafe.Pointer, key string) string {
	// Environment variables have priority
	envKey := "HSDP_" + strings.ToUpper(CamelCaseToUnderscore(key))
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

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "hsdp", "HSDP logging output plugin")
}

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	host := plugin.Environment(ctx, "IngestorHost")
	sharedKey := plugin.Environment(ctx, "SharedKey")
	secretKey := plugin.Environment(ctx, "SecretKey")
	productKey := plugin.Environment(ctx, "ProductKey")
	debug := plugin.Environment(ctx, "Debug")
	customField := plugin.Environment(ctx, "CustomField")

	var err error

	useCustomField = customField != "" // TODO: remove global

	client, err = logging.NewClient(nil,
		logging.Config{
			SharedKey:    sharedKey,
			SharedSecret: secretKey,
			ProductKey:   productKey,
			BaseURL:      host,
			Debug:        debug != "",
		})
	if err != nil {
		fmt.Printf("configuration errors: %v\n", err)
		plugin.Unregister(ctx)
		plugin.Exit(1)
		return output.FLB_ERROR
	}
	fmt.Printf("[out-hsdp] build:%s version:%s\n", builddate, revision)

	resources = make([]logging.Resource, batchSize)

	return output.FLB_OK
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func flushResources(resources []logging.Resource, count int) (*logging.StoreResponse, int, error) {
	var err error
	var resp *logging.StoreResponse
	maxLoop := count
	l := 0

	for {
		l++
		resp, err = client.StoreResources(resources, count)
		if err == nil { // Happy flow
			break
		}
		if err == logging.ErrBatchErrors && resp != nil {
			// Remove offending messages and resend
			nrErrors := len(resp.Failed)
			keys := make([]int, 0, nrErrors)
			for k := range resp.Failed {
				keys = append(keys, k)
			}
			pos := 0
			for i := 0; i < count; i++ {
				if contains(keys, i) {
					continue
				}
				resources[pos] = resources[i]
				pos++
			}
			count = pos
			fmt.Printf("[out-hsdp]: %d errors. resending %d\n", nrErrors, count)
		} else {
			fmt.Printf("[out-hsdp]: unexpected error in StoreResource(): %v\n", err)
		}
		// Break loop check
		if l > maxLoop || count <= 0 {
			fmt.Printf("[out-hsdp]: too many resends or nothing to send. giving up\n")
			break
		}
	}
	fmt.Printf("[out-hsdp] flushed %d/%d resources\n", count, maxLoop)
	return resp, count, err
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	// do something with the data
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}
	var count = 0
	var totalCount = 0
	var totalDelivered = 0

	// Create Fluent Bit decoder
	dec := plugin.NewDecoder(data, int(length))

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
			// DO NOT RETURN HERE because one message has an error when json is
			// generated, but a retry would fetch ALL messages again. instead an
			// error should be printed to console
			continue
		}
		resources[count] = *js
		count++
		totalCount++
		if count == batchSize {
			_, delivered, _ := flushResources(resources, count)
			totalDelivered += delivered
			count = 0
		}
	}
	if count > 0 {
		_, delivered, _ := flushResources(resources, count)
		totalDelivered += delivered
	}
	if totalDelivered == 0 {
		return output.FLB_RETRY
	}
	fmt.Printf("[out-hsdp] totals for this flush: %d/%d\n", totalDelivered, totalCount)
	return output.FLB_OK
}

func mapReturnDelete(m *map[string]interface{}, key, defaultValue string) string {
	outputVal := defaultValue
	if val, ok := (*m)[key].(string); ok && val != "" {
		outputVal = val
		delete(*m, key)
	}
	return outputVal
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
		case string:
			m[k.(string)] = v
		case []interface{}, map[interface{}]interface{}:
			m[k.(string)] = recursiveToJSON(v)
		default:
			m[k.(string)] = v
		}
	}
	id, _ := uuid.NewRandom()
	generatedTransactionID, _ := uuid.NewRandom()

	transactionID := mapReturnDelete(&m, "transaction_id", generatedTransactionID.String())
	if _, err := uuid.Parse(transactionID); err != nil { // validate
			transactionID = generatedTransactionID.String()
	}
	serverName := mapReturnDelete(&m, "server_name", "fluent-bit")
	appInstance := mapReturnDelete(&m, "app_instance", "fluent-bit")
	appName := mapReturnDelete(&m, "app_name", "fluent-bit")
	appVersion := mapReturnDelete(&m, "app_version", "1.0")
	component := mapReturnDelete(&m, "component", "fluent-bit")
	severity := mapReturnDelete(&m, "severity", "Informational")
	category := mapReturnDelete(&m, "category", "Tracelog")
	serviceName := mapReturnDelete(&m, "service_name", "fluent-bit")
	originatingUser := mapReturnDelete(&m, "originating_user", "fluent-bit")
	eventID := mapReturnDelete(&m, "event_id", "1")
	logMessage := mapReturnDelete(&m, "logdata_message", "")

	msg, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error creating message for hsdp-logging: %v", err)
	}
	if logMessage == "" {
		logMessage = string(msg)
	}
	resource := &logging.Resource{
		ID:                  id.String(),
		Severity:            severity,
		ApplicationInstance: appInstance,
		ApplicationName:     appName,
		OriginatingUser:     originatingUser,
		Category:            category,
		Component:           component,
		ApplicationVersion:  appVersion,
		ServerName:          serverName,
		ServiceName:         serviceName,
		EventID:             eventID,
		TransactionID:       transactionID,
		LogTime:             timestamp.UTC().Format(logging.TimeFormat),
		LogData:             logging.LogData{Message: logMessage},
	}
	if useCustomField {
		resource.Custom = msg
	}
	return resource, nil
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}
