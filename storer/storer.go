package storer

import (
	"github.com/philips-software/go-hsdp-api/logging"
)

type Storer interface {
	StoreResources(messages []logging.Resource, count int) (*logging.StoreResponse, error)
}
