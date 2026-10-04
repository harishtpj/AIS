APP := ais
GRUBY_DIR := vendors/gruby

.PHONY: all build gruby clean test run

all: build

build: gruby
	go build ./cmd/ais

gruby:
	$(MAKE) -C $(GRUBY_DIR) libmruby.a test

test: gruby
	go test ./...

run: gruby
	go run ./cmd/ais start -c configs/ais.rb

clean:
	rm -f $(APP)
	$(MAKE) -C $(GRUBY_DIR) clean
	go clean