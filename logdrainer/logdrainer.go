package logdrainer

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/influxdata/go-syslog/v2/rfc5424"
	"github.com/philips-software/fluent-bit-out-hsdp/storer"
	"github.com/philips-software/go-hsdp-api/logging"
)

type logDrainerStorer struct {
	*http.Client
	logDrainerURL   *url.URL
	applicationName string
	serverName      string
	debug           bool
}

func (l *logDrainerStorer) StoreResources(messages []logging.Resource, count int) (*logging.StoreResponse, error) {
	var resp *http.Response
	logResponse := &logging.StoreResponse{}

	for i := 0; i < count; i++ {
		var err error
		msg := messages[i]
		decoded, err := base64.StdEncoding.DecodeString(msg.LogData.Message)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to decode message: %v", err)
			continue
		}
		syslogMessage := rfc5424.SyslogMessage{}
		syslogMessage.SetTimestamp(time.Now().Format(time.RFC3339))
		syslogMessage.SetPriority(14)
		syslogMessage.SetVersion(1)
		syslogMessage.SetProcID("[APP/PROC/FLUENT-BIT-OUT-HSDP/0]")
		syslogMessage.SetAppname(msg.ApplicationName)
		if l.applicationName != "" {
			syslogMessage.SetAppname(l.applicationName)
		}
		if l.serverName != "" {
			syslogMessage.SetHostname(fmt.Sprintf("%s.fluent-bit.%s", l.serverName, l.applicationName))
		}
		syslogMessage.SetParameter("fluent-bit-out-hsdp", "taskId", msg.ApplicationInstance)
		syslogMessage.SetParameter("fluent-bit-out-hsdp", "applicationName", l.applicationName)
		syslogMessage.SetParameter("fluent-bit-out-hsdp", "serverName", l.serverName)
		syslogMessage.SetMessage(string(decoded))
		if msg.TraceID != "" || msg.SpanID != "" { // Construct a CustomLogEvent to propagate these
			customLogEvent := fmt.Sprintf("%s|CustomLogEvent|%s|%s|%s|%s|%s", msg.Severity, msg.TransactionID, msg.TraceID, msg.SpanID, msg.Component, string(decoded))
			syslogMessage.SetMessage(customLogEvent)
		}
		message, _ := syslogMessage.String()
		if l.debug {
			fmt.Printf("[out-hsdp] RFC5424: %s\n", message)
		}
		req := &http.Request{
			Method: http.MethodPost,
			URL:    l.logDrainerURL,
			Header: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		}
		req.Body = io.NopCloser(strings.NewReader(message))
		resp, err = l.Client.Do(req)
		if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
			_, _ = fmt.Fprintf(os.Stderr, "failed to send log: %v %v", resp, err)
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
	logResponse.Response = &http.Response{StatusCode: http.StatusOK}
	return logResponse, nil
}

func NewStorer(logDrainerURL string, opts ...OptionFunc) (storer.Storer, error) {
	if logDrainerURL == "" {
		return nil, fmt.Errorf("missing or empty logDrainerURL")
	}
	parsedURL, err := url.Parse(logDrainerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL '%s': %v", logDrainerURL, err)
	}
	s := &logDrainerStorer{
		Client:        &http.Client{},
		logDrainerURL: parsedURL,
	}
	for _, o := range opts {
		if o(s) != nil {
			return nil, err
		}
	}
	return s, nil
}
