package storer

import (
	"github.com/dip-software/go-dip-api/logging"
)

type Storer interface {
	StoreResources(messages []logging.Resource, count int) (*logging.StoreResponse, error)
}
