package hsdp

import (
	"C"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/google/uuid"
	"github.com/philips-software/go-hsdp-api/iam"
	"github.com/philips-software/go-hsdp-api/logging"
)

var (
	plugin         Plugin = &fluentPlugin{}
	client         storer
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

type storer interface {
	StoreResources(messages []logging.Resource, count int) (*logging.StoreResponse, error)
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
	return output.NewDecoder(data, length)
}

func (p *fluentPlugin) Exit(code int) {
	os.Exit(code)
}

func (p *fluentPlugin) Send(_ []logging.Resource) error {
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
	serviceID := plugin.Environment(ctx, "ServiceId")
	servicePrivateKey := plugin.Environment(ctx, "ServicePrivateKey")
	productKey := plugin.Environment(ctx, "ProductKey")
	debugging := plugin.Environment(ctx, "Debug")
	customField := plugin.Environment(ctx, "CustomField")
	noTLS := plugin.Environment(ctx, "InsecureSkipVerify")
	idmURL := plugin.Environment(ctx, "IdmUrl")
	iamURL := plugin.Environment(ctx, "IamUrl")
	logdrainURL := plugin.Environment(ctx, "LogdrainUrl")

	var err error

	useCustomField = customField == "true" || customField == "yes" || customField == "1" // TODO: remove global
	ignoreTLS = noTLS == "true" || noTLS == "yes" || noTLS == "1"
	enableDebug := debugging == "true" || debugging == "yes" || debugging == "1"

	c := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	if ignoreTLS {
		fmt.Printf("InsecureSkipVerify: %v\n", ignoreTLS)
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: ignoreTLS,
			},
		}
		c.Transport = tr
	}

	config := &logging.Config{
		Region:      region,
		Environment: environment,

		ProductKey: productKey,
		Debug:      enableDebug,
	}

	if host != "" {
		config.BaseURL = host
	}

	validCreds := false
	if sharedKey != "" && secretKey != "" {
		config.SharedKey = sharedKey
		config.SharedSecret = secretKey
		validCreds = true
	}
	if serviceID != "" && servicePrivateKey != "" {
		debugLog := ""
		if enableDebug {
			debugLog = "/dev/stderr"
		}
		iamClient, err := iam.NewClient(nil, &iam.Config{
			Region:      region,
			Environment: environment,
			IDMURL:      idmURL,
			IAMURL:      iamURL,
			DebugLog:    debugLog,
		})
		if err != nil {
			fmt.Printf("[out-hsdp] invalid service credentials: %v\n", err)
			plugin.Exit(1)
			return output.FLB_ERROR
		}
		err = iamClient.ServiceLogin(iam.Service{
			ServiceID:  serviceID,
			PrivateKey: servicePrivateKey,
		})
		if err != nil {
			fmt.Printf("[out-hsdp] invalid service credentials: %v\n", err)
			plugin.Exit(1)
			return output.FLB_ERROR
		}
		// TODO: maybe add a scopes check here
		config.IAMClient = iamClient
		config.SharedKey = ""
		config.SharedSecret = ""
		validCreds = true
		fmt.Printf("[out-hsdp] using service credentials\n")
	}
	if logdrainURL != "" {
		validCreds = true
		fmt.Printf("[out-hsdp] using logdrain endpoint\n")
	}
	if !validCreds {
		fmt.Printf("[out-hsdp] no valid credentials found\n")
		plugin.Exit(1)
		return output.FLB_ERROR
	}

	if logdrainURL != "" {
		client, err = NewLogDrainerStorer(logdrainURL)
		if err != nil {
			fmt.Printf("[out-hsdp] configuration error: %v\n", err)
			plugin.Unregister(ctx)
			plugin.Exit(1)
			return output.FLB_ERROR
		}
	} else {
		client, err = logging.NewClient(c, config)
		if err != nil {
			fmt.Printf("[out-hsdp] configuration error: %v\n", err)
			plugin.Unregister(ctx)
			plugin.Exit(1)
			return output.FLB_ERROR
		}
	}
	info, ok := debug.ReadBuildInfo()
	if ok {
		fmt.Printf("[out-hsdp] build: %s\n", info.String())
	}

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
					resp, err := flushResources(resources, count)
					if err != nil {
						printError(resp, err)
					}
					count = 0
				}
			case <-time.After(1 * time.Second):
				if count > 0 {
					resp, err := flushResources(resources, count)
					if err != nil {
						printError(resp, err)
					}
					count = 0
				}
			}
		}
	}()

	return output.FLB_OK
}

func printError(resp *logging.StoreResponse, err error) {
	fmt.Printf("[out-hsdp] flush error: %v\n", err)
	if resp.Response != nil {
		fmt.Printf("[out-hsdp] response: %v\n", resp.Response)
	}
	for i, msg := range resp.Failed {
		data, err := json.Marshal(msg)
		if err != nil {
			data = []byte(fmt.Sprintf("decoding error: %v", err))
		}
		fmt.Printf("[out-hsdp] error entry %d: [%v] [%v]\n", i, msg.Error, string(data))
	}
}

func flushResources(resources []logging.Resource, count int) (*logging.StoreResponse, error) {
	return client.StoreResources(resources, count)
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
			fmt.Print("[out-hsdp] given time is not in a known format, defaulting to now.\n")
			timeStamp = time.Now()
		}

		js, err := createResource(timeStamp, C.GoString(tag), record)
		if err != nil {
			fmt.Printf("%v\n", err)
			// DO NOT RETURN HERE because one message has an error when json is
			// generated, but a retry would fetch ALL messages again. Instead, an
			// error should be printed to console
			continue
		}
		queue <- *js
	}
	return output.FLB_OK
}

func mapReturnDelete(m *map[string]interface{}, key, defaultValue string) string {
	out := defaultValue
	if val, ok := (*m)[key].(string); ok && val != "" {
		out = val
		delete(*m, key)
	}
	return out
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
		return nil, fmt.Errorf("[out-hsdp] error creating message for hsdp-logging: %v", err)
	}

	var resource logging.Resource

	// Do we have a native LogEvent?
	if err = json.Unmarshal(msg, &resource); err == nil && resource.Valid() {
		return &resource, nil
	}

	id, _ := uuid.NewRandom()
	generatedTransactionID, _ := uuid.NewRandom()

	transactionID := mapReturnDelete(&m, "transaction_id", generatedTransactionID.String())
	if parsed, err := uuid.Parse(transactionID); err != nil { // validate
		transactionID = generatedTransactionID.String()
	} else {
		transactionID = parsed.String() // sanitized version
	}
	serverName := mapReturnDelete(&m, "server_name", "fluent-bit")
	appInstance := mapReturnDelete(&m, "app_instance", tag)
	appName := mapReturnDelete(&m, "app_name", "fluent-bit")
	appVersion := mapReturnDelete(&m, "app_version", "1.0")
	component := mapReturnDelete(&m, "component", "fluent-bit")
	severity := mapReturnDelete(&m, "severity", "Informational")
	category := mapReturnDelete(&m, "category", "TraceLog")
	serviceName := mapReturnDelete(&m, "service_name", tag)
	originatingUser := mapReturnDelete(&m, "originating_user", "fluent-bit")
	eventID := mapReturnDelete(&m, "event_id", "1")
	logMessage := mapReturnDelete(&m, "logdata_message", "")
	traceID := mapReturnDelete(&m, "trace_id", "")
	spanID := mapReturnDelete(&m, "span_id", "")

	msg, err = json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error creating message for hsdp-logging: %v", err)
	}

	custom := json.RawMessage{}
	if logMessage == "" {
		logMessage = string(msg)
	}

	logMessage = strings.ReplaceAll(logMessage, "\\u2028", "\n")

	// Base64 encode. Requires go-hsdp-api 0.49.1+
	logMessage = base64.StdEncoding.EncodeToString([]byte(logMessage))

	resource = logging.Resource{
		ID:                  id.String(),
		ResourceType:        "LogEvent",
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
		TraceID:             traceID,
		SpanID:              spanID,
		LogTime:             timestamp.UTC().Format(logging.TimeFormat),
		LogData:             logging.LogData{Message: logMessage},
		Custom:              custom,
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
