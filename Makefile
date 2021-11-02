SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --always)
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	BUILDDATE := $(shell date --rfc-3339=seconds)
endif
ifeq ($(UNAME_S),Darwin)
	BUILDDATE := $(shell gdate --rfc-3339=seconds)
endif

all: test
	go build -ldflags "-X 'main.revision=$(GITVERSION)' -X 'main.buildDate=$(BUILDDATE)'" -buildmode=c-shared -o out_hsdp.so .

test:
	go test -cover -race -coverprofile=coverage.txt -covermode=atomic

clean:
	rm -rf *.so *.h *~
