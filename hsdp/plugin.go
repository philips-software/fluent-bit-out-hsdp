package hsdp

import (
	"C"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
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
	revision       string
	builddate      string
	plugin         Plugin = &fluentPlugin{}
	client         *logging.Client
	queue          chan logging.Resource
	useCustomField bool
	ignoreTLS      bool
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
	Send(values []logging.Resource) error
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
	region := plugin.Environment(ctx, "Region")
	environment := plugin.Environment(ctx, "Environment")
	host := plugin.Environment(ctx, "IngestorHost")
	sharedKey := plugin.Environment(ctx, "SharedKey")
	secretKey := plugin.Environment(ctx, "SecretKey")
	productKey := plugin.Environment(ctx, "ProductKey")
	debug := plugin.Environment(ctx, "Debug")
	customField := plugin.Environment(ctx, "CustomField")
	noTLS := plugin.Environment(ctx, "IgnoreTLS")

	var err error

	useCustomField = customField != "" // TODO: remove global
	ignoreTLS = noTLS != ""

	c := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	if ignoreTLS {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		c.Transport = tr
	}

	client, err = logging.NewClient(c,
		&logging.Config{
			Region:       region,
			Environment:  environment,
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

	queue = make(chan logging.Resource)

	go func() {
		var count int
		resources := make([]logging.Resource, batchSize)

		for {
			select {
			case r := <-queue:
				// append and send
				resources[count] = r
				count++
				if count == batchSize {
					flushResources(resources, count)
					count = 0
				}
			case <-time.After(1 * time.Second):
				if count > 0 {
					flushResources(resources, count)
					count = 0
				}
			}
		}
	}()

	return output.FLB_OK
}

func flushResources(resources []logging.Resource, count int) error {
	fmt.Printf("[out-hsdp] flushing %d resources\n", count)
	_, err := client.StoreResources(resources, count)
	return err
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	// do something with the data
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}

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
			// DO NOT RETURN HERE becase one message has an error when json is
			// generated, but a retry would fetch ALL messages again. instead an
			// error should be printed to console
			continue
		}
		queue <- *js
	}
	return output.FLB_OK
}

func mapReturnDelete(m *map[string]interface{}, key, defaultValue string) string {
	output := defaultValue
	if val, ok := (*m)[key].(string); ok && val != "" {
		output = val
		delete(*m, key)
	}
	return output
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
	msg, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error creating message for hsdp-logging: %v", err)
	}

	var resource logging.Resource

	// Do we have a native LogEvent?
	if err = json.Unmarshal(msg, &resource); err == nil && resource.Valid() {
		return &resource, nil
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

	if logMessage == "" {
		logMessage = string(msg)
	}
	resource = logging.Resource{
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

	return &resource, nil
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}
